package fabric

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"
)

func skipIfNoNetwork(t *testing.T) {
	t.Helper()
	if os.Getenv("FABRIC_INTEGRATION") != "1" {
		t.Skip("set FABRIC_INTEGRATION=1 to run against the live Fabric network")
	}
}

func uniqueElectionID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-election-%d", time.Now().UnixNano())
}

func setup(t *testing.T) (*Gateway, NetworkConfig) {
	t.Helper()

	time.Sleep(1 * time.Second)

	cfg := DefaultConfig()
	gw, err := Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect to Fabric: %v", err)
	}
	t.Cleanup(gw.Close)
	return gw, cfg
}

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

func startVoting(t *testing.T, gw *Gateway, electionID string) {
	t.Helper()
	_, err := gw.Contract.SubmitTransaction("StartVoting", electionID)
	if err != nil {
		t.Fatalf("StartVoting failed: %v", err)
	}
}

func submitBallot(t *testing.T, gw *Gateway, electionID, ballotID, encVotes, token string) error {
	t.Helper()
	_, err := gw.Contract.SubmitTransaction("SubmitBallot", electionID, ballotID, encVotes, token)
	return err
}

// makeEncryptedVotes produces a JSON array of ciphertexts, one "C1,C2" pair
// per candidate.
func makeEncryptedVotes(c1s, c2s []string) string {
	var votes []string
	for i := range c1s {
		b, _ := json.Marshal(map[string]string{"C1": c1s[i], "C2": c2s[i]})
		votes = append(votes, string(b))
	}
	out, _ := json.Marshal(votes)
	return string(out)
}

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

	// Cast 10 ballots: 6 for A, 4 for B
	const numVotes = 10
	for i := 0; i < numVotes; i++ {
		var encVotes string
		if i < 6 {
			encVotes = makeEncryptedVotes([]string{"100", "150"}, []string{"200", "250"})
		} else {
			encVotes = makeEncryptedVotes([]string{"150", "100"}, []string{"250", "200"})
		}
		err := submitBallot(t, gw, electionID, fmt.Sprintf("ballot-%d", i), encVotes, fmt.Sprintf("token-%d", i))
		if err != nil {
			t.Fatalf("SubmitBallot %d failed: %v", i, err)
		}
	}

	if _, err := gw.Contract.SubmitTransaction("FreezeElection", electionID); err != nil {
		t.Fatalf("FreezeElection failed: %v", err)
	}

	tally := evaluateTally(t, gw, electionID)

	gotCount := int64(tally["ballotCount"].(float64))
	if gotCount != numVotes {
		t.Errorf("expected %d ballots, got %d", numVotes, gotCount)
	}

	candidates, ok := tally["candidates"].([]interface{})
	if !ok || len(candidates) != 2 {
		t.Fatalf("expected 2 candidates in tally, got %v", tally["candidates"])
	}

	c1s, ok := tally["c1s"].([]interface{})
	if !ok || len(c1s) != 2 {
		t.Fatalf("expected 2 C1s, got %v", tally["c1s"])
	}

	// Candidate A: 6 votes of C1=100 and 4 votes of C1=150
	// C1_A = 100^6 * 150^4
	expectedA := new(big.Int).Exp(big.NewInt(100), big.NewInt(6), nil)
	expectedA.Mul(expectedA, new(big.Int).Exp(big.NewInt(150), big.NewInt(4), nil))
	gotA, _ := new(big.Int).SetString(c1s[0].(string), 10)
	if gotA.Cmp(expectedA) != 0 {
		t.Errorf("C1_A mismatch: got %s, want %s", gotA, expectedA)
	}

	t.Logf("tally: count=%d, C1_A matches expected (%d bytes)", gotCount, len(gotA.String()))
}

func TestDoubleSpendRejected(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	electionID := uniqueElectionID(t)
	t.Logf("electionID = %s", electionID)
	registerElection(t, gw, electionID)
	startVoting(t, gw, electionID)

	enc := makeEncryptedVotes([]string{"10", "20"}, []string{"30", "40"})
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

	enc := makeEncryptedVotes([]string{"1", "2"}, []string{"3", "4"})
	if err := submitBallot(t, gw, electionID, "ballot-1", enc, "token-1"); err != nil {
		t.Fatalf("SubmitBallot should succeed: %v", err)
	}

	if _, err := gw.Contract.SubmitTransaction("FreezeElection", electionID); err != nil {
		t.Fatalf("FreezeElection failed: %v", err)
	}

	err := submitBallot(t, gw, electionID, "ballot-2", enc, "token-2")
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

	enc := makeEncryptedVotes([]string{"1", "2"}, []string{"3", "4"})
	err := submitBallot(t, gw, electionID, "ballot-1", enc, "token-1")
	if err == nil {
		t.Fatal("SubmitBallot in REGISTRATION phase should fail")
	}
	t.Logf("correctly rejected pre-voting submission: %v", err)
}

func TestGetAllBallots(t *testing.T) {
	skipIfNoNetwork(t)
	gw, _ := setup(t)

	electionID := uniqueElectionID(t)
	t.Logf("electionID = %s", electionID)
	registerElection(t, gw, electionID)
	startVoting(t, gw, electionID)

	for i := 0; i < 3; i++ {
		enc := makeEncryptedVotes([]string{"10", "20"}, []string{"30", "40"})
		if err := submitBallot(t, gw, electionID, fmt.Sprintf("b%d", i), enc, fmt.Sprintf("t%d", i)); err != nil {
			t.Fatalf("SubmitBallot %d failed: %v", i, err)
		}
	}

	result, err := gw.Contract.EvaluateTransaction("GetAllBallots", electionID)
	if err != nil {
		t.Fatalf("GetAllBallots failed: %v", err)
	}

	var ballots []map[string]interface{}
	if err := json.Unmarshal(result, &ballots); err != nil {
		t.Fatalf("ballots JSON parse: %v (raw: %s)", err, result)
	}
	if len(ballots) != 3 {
		t.Errorf("expected 3 ballots, got %d", len(ballots))
	}
	for _, b := range ballots {
		votes, ok := b["encryptedVotes"].([]interface{})
		if !ok || len(votes) != 2 {
			t.Errorf("ballot %v has wrong encryptedVotes", b["ballotId"])
		}
	}
}
