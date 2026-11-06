package tally

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/blockchain-voting-system/pkg/client"
	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
	"github.com/blockchain-voting-system/pkg/tally"
)

// TestTallyEndToEnd is the Milestone 4 capstone: full ceremony from voter
// registration to verified decrypted result.
func TestTallyEndToEnd(t *testing.T) {
	identityURL := Env(t, "IDENTITY_URL")
	castingURL := Env(t, "CASTING_URL")
	tallyURL := Env(t, "TALLY_URL")

	runID := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	t.Logf("runID = %s", runID)

	// 1. Generate ceremony
	_, pubKey := ProvisionCeremony(t, runID)

	// 2. Connect to Fabric for provisioning + verification
	fab := NewFabricClient(t)
	defer fab.Close()

	// 3. Provision election
	electionID := ProvisionElection(t, fab, pubKey)

	// 4. Cast 15 ballots (10 Alice, 5 Bob) via voting-service
	votingClient := client.NewClient(identityURL, castingURL, pubKey)

	const numBallots = 15
	const aliceVotes = 10
	aliceCount, bobCount := 0, 0

	for i := 0; i < numBallots; i++ {
		voterID := fmt.Sprintf("voter-%03d", i+1)

		// Register
		reg, err := votingClient.Register(voterID, electionID)
		if err != nil {
			t.Fatalf("register voter %s: %v", voterID, err)
		}

		// Cast
		var votes []int64
		if i < aliceVotes {
			votes = []int64{1, 0}
			aliceCount++
		} else {
			votes = []int64{0, 1}
			bobCount++
		}
		if _, err := votingClient.Cast(electionID, reg.Credential, votes); err != nil {
			t.Fatalf("cast ballot %d: %v", i, err)
		}
	}
	t.Logf("cast %d ballots (%d Alice, %d Bob)", numBallots, aliceCount, bobCount)

	// 5. Freeze the election
	fab.Submit(t, "FreezeElection", electionID)
	t.Log("election frozen")

	// 6. Run the tally ceremony via HTTP
	tallyHTTP := NewHTTPClient(tallyURL)
	var result tally.Result
	code, err := tallyHTTP.PostJSON(
		"/tally/"+electionID,
		map[string]string{"ceremonyDir": ContainerCeremonyDir(runID)},
		&result,
	)
	if err != nil {
		t.Fatalf("tally HTTP call: %v", err)
	}
	if code != 200 {
		t.Fatalf("tally: HTTP %d", code)
	}

	// 7. Verify the result
	if result.ElectionID != electionID {
		t.Errorf("electionID mismatch: %s vs %s", result.ElectionID, electionID)
	}
	if result.TotalBallots != numBallots {
		t.Errorf("totalBallots: got %d, want %d", result.TotalBallots, numBallots)
	}
	if len(result.CandidateResults) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(result.CandidateResults))
	}

	sums := map[string]int64{}
	for _, r := range result.CandidateResults {
		sums[r.Candidate] = r.Sum
	}
	if sums["Alice"] != aliceVotes {
		t.Errorf("Alice: got %d, want %d", sums["Alice"], aliceVotes)
	}
	if sums["Bob"] != int64(bobCount) {
		t.Errorf("Bob: got %d, want %d", sums["Bob"], bobCount)
	}

	// 8. Verify ZK proofs
	for _, r := range result.CandidateResults {
		if r.Proof == nil {
			t.Errorf("candidate %q has no proof", r.Candidate)
			continue
		}
		// The proof proves that the ciphertext decrypts to g^sum.
		// We need to fetch the encrypted tally to verify against it.
		// For simplicity, just verify the proof is well-formed (non-nil fields).
		if r.Proof.A == nil || r.Proof.B == nil || r.Proof.S == nil {
			t.Errorf("candidate %q proof has nil fields", r.Candidate)
		}
	}

	// 9. Verify GetTally from chaincode
	tallyRaw := fab.Evaluate(t, "GetTally", electionID)
	var chaincodeTally struct {
		ElectionID       string `json:"electionId"`
		CandidateResults []struct {
			Candidate string `json:"candidate"`
			Sum       int64  `json:"sum"`
			Proof     string `json:"proof"`
		} `json:"candidateResults"`
		TotalBallots int64 `json:"totalBallots"`
	}
	if err := json.Unmarshal(tallyRaw, &chaincodeTally); err != nil {
		t.Fatalf("parse GetTally: %v", err)
	}
	if chaincodeTally.TotalBallots != numBallots {
		t.Errorf("chaincode tally: got %d ballots, want %d", chaincodeTally.TotalBallots, numBallots)
	}

	// 10. Verify election phase is TALLIED
	electionRaw := fab.Evaluate(t, "GetElection", electionID)
	var election struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(electionRaw, &election); err != nil {
		t.Fatalf("parse GetElection: %v", err)
	}
	if election.Phase != "TALLIED" {
		t.Errorf("phase: got %s, want TALLIED", election.Phase)
	}

	t.Logf("E2E success: election=%s, ballots=%d, Alice=%d, Bob=%d, phase=%s",
		electionID, numBallots, sums["Alice"], sums["Bob"], election.Phase)
}

// nowNanos returns UnixNano as int64, isolated here so we can override in tests.
//func nowNanos() int64 {
//	return time.Now().UnixNano()
//}

// Full proof verification test (a more thorough version).
func TestTallyProofsVerify(t *testing.T) {
	identityURL := Env(t, "IDENTITY_URL")
	castingURL := Env(t, "CASTING_URL")
	tallyURL := Env(t, "TALLY_URL")

	runID := fmt.Sprintf("proof-%d", time.Now().UnixNano())
	t.Logf("runID = %s", runID)

	ceremonyDir, pubKey := ProvisionCeremony(t, runID)

	fab := NewFabricClient(t)
	defer fab.Close()

	electionID := ProvisionElection(t, fab, pubKey)

	// Cast 5 ballots (3 Alice, 2 Bob)
	votingClient := client.NewClient(identityURL, castingURL, pubKey)
	for i := 0; i < 5; i++ {
		voterID := fmt.Sprintf("voter-%03d", i+1)
		reg, err := votingClient.Register(voterID, electionID)
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		votes := []int64{1, 0}
		if i >= 3 {
			votes = []int64{0, 1}
		}
		if _, err := votingClient.Cast(electionID, reg.Credential, votes); err != nil {
			t.Fatalf("cast: %v", err)
		}
	}

	fab.Submit(t, "FreezeElection", electionID)

	// Fetch the encrypted tally (public data)
	tallyRaw := fab.Evaluate(t, "GetEncryptedTally", electionID)
	var encTally struct {
		Candidates []string `json:"candidates"`
		C1s        []string `json:"c1s"`
		C2s        []string `json:"c2s"`
	}
	if err := json.Unmarshal(tallyRaw, &encTally); err != nil {
		t.Fatalf("parse enc tally: %v", err)
	}

	// Run tally
	tallyHTTP := NewHTTPClient(tallyURL)
	var result tally.Result
	code, err := tallyHTTP.PostJSON(
		"/tally/"+electionID,
		map[string]string{"ceremonyDir": ContainerCeremonyDir(runID)},
		&result,
	)
	if err != nil || code != 200 {
		t.Fatalf("tally: HTTP %d, err=%v", code, err)
	}

	// Verify each proof against the encrypted ciphertext
	for i, r := range result.CandidateResults {
		c1, _ := new(big.Int).SetString(encTally.C1s[i], 10)
		c2, _ := new(big.Int).SetString(encTally.C2s[i], 10)

		// Proof claims the ciphertext decrypts to g^sum
		m := new(big.Int).Exp(pubKey.G, big.NewInt(r.Sum), pubKey.P)

		if !zkproofs.VerifyDecryptionProof(c1, c2, m, pubKey.Y, pubKey.P, pubKey.G, r.Proof) {
			t.Errorf("proof %d (%s) failed verification", i, r.Candidate)
		} else {
			t.Logf("proof %d (%s, sum=%d) verified", i, r.Candidate, r.Sum)
		}
	}

	_ = ceremonyDir
}

// Ensure imports are used.
var _ = elgamal.PublicKey{}
