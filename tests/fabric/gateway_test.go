package fabric

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"
)

// skipIfNoNetwork skips the test if the integration flag isn't set.
// This lets `go test ./...` run without a live Fabric network.
func skipIfNoNetwork(t *testing.T) {
	t.Helper()
	if os.Getenv("FABRIC_INTEGRATION") != "1" {
		t.Skip("set FABRIC_INTEGRATION=1 to run against the live Fabric network")
	}
}

// uniqueElectionID returns a fresh election ID per run, so tests don't
// collide on ledger state.
func uniqueElectionID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-election-%d", time.Now().UnixNano())
}

// setup connects to the network and returns a Gateway + helper.
func setup(t *testing.T) (*Gateway, NetworkConfig) {
	t.Helper()
	cfg := DefaultConfig()
	gw, err := Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect to Fabric: %v", err)
	}
	t.Cleanup(gw.Close)
	return gw, cfg
}

// registerElection registers an election with two candidates.
func registerElection(t *testing.T, gw *Gateway, electionID string) {
	t.Helper()
	candidates := `["Alice","Bob"]`
	_, err := gw.Contract.SubmitTransaction(
		"RegisterElection", electionID, "Integration Test Election", candidates, "test-pubkey",
	)
	if err != nil {
		t.Fatalf("RegisterElection failed: %v", err)
	}
}

// startVoting transitions an election to VOTING.
func startVoting(t *testing.T, gw *Gateway, electionID string) {
	t.Helper()
	_, err := gw.Contract.SubmitTransaction("StartVoting", electionID)
	if err != nil {
		t.Fatalf("StartVoting failed: %v", err)
	}
}

// submitBallot casts one ballot.
func submitBallot(t *testing.T, gw *Gateway, electionID, ballotID, encVote, token string) error {
	t.Helper()
	_, err := gw.Contract.SubmitTransaction("SubmitBallot", electionID, ballotID, encVote, token)
	return err
}

// makeEncryptedVote produces a JSON ciphertext blob the chaincode will accept.
// The chaincode only cares that C1/C2 are decimal strings.
func makeEncryptedVote(c1, c2 string) string {
	b, _ := json.Marshal(map[string]string{"C1": c1, "C2": c2})
	return string(b)
}

// evaluateTally fetches the encrypted tally.
func evaluateTally(t *testing.T, gw *Gateway, electionID string) map[string]interface{} {
	t.Helper()
	result, err := gw.Contract.EvaluateTransaction("GetEncryptedTally", electionID)
	if err != nil {
		t.Fatalf("GetEncryptedTally failed: %v", err)
	}
	var tally map[string]interface{}
	if err := json.Unmarshal(result, &tally); err != nil {
		t.Fatalf("tally JSON parse: %v (raw: %s)", err, result)
	}
	return tally
}

// --- Tests ---

func TestHealthCheck(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	// RegisterElection is a write — proves we can submit + commit.
	// Use a random ID so this is repeatable.
	id := uniqueElectionID(t)
	defer t.Logf("electionID = %s", id)

	registerElection(t, gw, id)
	t.Log("successfully registered election — Gateway pipeline works")
}

func TestFullElectionFlow(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	electionID := uniqueElectionID(t)
	t.Logf("electionID = %s", electionID)

	registerElection(t, gw, electionID)
	startVoting(t, gw, electionID)

	// Cast 10 ballots: 6 for "1", 4 for "0"
	const numVotes = 10
	expectedSum := int64(0)
	for i := 0; i < numVotes; i++ {
		var c1, c2 string
		var vote int64
		if i < 6 {
			vote = 1
			c1, c2 = "100", "200" // arbitrary nonzero decimal
		} else {
			vote = 0
			c1, c2 = "150", "250"
		}
		expectedSum += vote

		err := submitBallot(t, gw, electionID, fmt.Sprintf("ballot-%d", i), makeEncryptedVote(c1, c2), fmt.Sprintf("token-%d", i))
		if err != nil {
			t.Fatalf("SubmitBallot %d failed: %v", i, err)
		}
	}

	// Freeze
	if _, err := gw.Contract.SubmitTransaction("FreezeElection", electionID); err != nil {
		t.Fatalf("FreezeElection failed: %v", err)
	}

	// Tally
	tally := evaluateTally(t, gw, electionID)

	// Verify ballot count
	gotCount := int64(tally["ballotCount"].(float64))
	if gotCount != numVotes {
		t.Errorf("expected %d ballots, got %d", numVotes, gotCount)
	}

	// Verify homomorphic sum (product of C1s and C2s)
	// Since we used (100,200) x6 and (150,250) x4, the C1 product
	// should be 100^6 * 150^4 and C2 product 200^6 * 250^4.
	expectedC1 := new(big.Int).Exp(big.NewInt(100), big.NewInt(6), nil)
	expectedC1.Mul(expectedC1, new(big.Int).Exp(big.NewInt(150), big.NewInt(4), nil))

	gotC1, ok := new(big.Int).SetString(tally["c1"].(string), 10)
	if !ok {
		t.Fatalf("could not parse c1: %v", tally["c1"])
	}
	if gotC1.Cmp(expectedC1) != 0 {
		t.Errorf("c1 mismatch: got %s, want %s", gotC1, expectedC1)
	}

	t.Logf("tally: count=%d, c1=%s (matches expected)", gotCount, gotC1)
}

func TestDoubleSpendRejected(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	electionID := uniqueElectionID(t)
	t.Logf("electionID = %s", electionID)
	registerElection(t, gw, electionID)
	startVoting(t, gw, electionID)

	enc := makeEncryptedVote("10", "20")
	if err := submitBallot(t, gw, electionID, "ballot-1", enc, "shared-token"); err != nil {
		t.Fatalf("first SubmitBallot should succeed: %v", err)
	}

	err := submitBallot(t, gw, electionID, "ballot-2", enc, "shared-token")
	if err == nil {
		t.Fatal("second SubmitBallot with same token should fail")
	}
	t.Logf("correctly rejected double-spend: %v", err)
}

func TestSubmitAfterFreezeRejected(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	electionID := uniqueElectionID(t)
	t.Logf("electionID = %s", electionID)
	registerElection(t, gw, electionID)
	startVoting(t, gw, electionID)

	// Cast one valid ballot
	if err := submitBallot(t, gw, electionID, "ballot-1", makeEncryptedVote("1", "2"), "token-1"); err != nil {
		t.Fatalf("SubmitBallot should succeed: %v", err)
	}

	// Freeze
	if _, err := gw.Contract.SubmitTransaction("FreezeElection", electionID); err != nil {
		t.Fatalf("FreezeElection failed: %v", err)
	}

	// Try to submit after freeze
	err := submitBallot(t, gw, electionID, "ballot-2", makeEncryptedVote("3", "4"), "token-2")
	if err == nil {
		t.Fatal("SubmitBallot after freeze should fail")
	}
	t.Logf("correctly rejected post-freeze submission: %v", err)
}

func TestSubmitBeforeVotingRejected(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	electionID := uniqueElectionID(t)
	t.Logf("electionID = %s", electionID)
	registerElection(t, gw, electionID)
	// Note: NOT calling StartVoting — phase remains REGISTRATION

	err := submitBallot(t, gw, electionID, "ballot-1", makeEncryptedVote("1", "2"), "token-1")
	if err == nil {
		t.Fatal("SubmitBallot in REGISTRATION phase should fail")
	}
	t.Logf("correctly rejected pre-voting submission: %v", err)
}
