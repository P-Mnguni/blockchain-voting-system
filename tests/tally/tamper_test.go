package tally

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/blockchain-voting-system/pkg/client"
	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
	"github.com/blockchain-voting-system/pkg/keyceremony"
)

// TestProvisionTamperedElection creates an election where the operator
// published wrong sums with real proofs attached. The verifier will detect
// the mismatch.
//
// Run with SKIP_TALLY=1 to provision and stop (for the demo recording):
//
//	SKIP_TALLY=1 go test ./test/tally/... -run TestProvisionTamperedElection -v
func TestProvisionTamperedElection(t *testing.T) {
	identityURL := Env(t, "IDENTITY_URL")
	castingURL := Env(t, "CASTING_URL")

	runID := fmt.Sprintf("tamper-%d", time.Now().UnixNano())
	t.Logf("runID = %s", runID)

	// 1. Generate ceremony
	_, pubKey := ProvisionCeremony(t, runID)

	// 2. Provision election
	fab := NewFabricClient(t)
	defer fab.Close()
	electionID := ProvisionElection(t, fab, pubKey)

	// 3. Cast 5 ballots: 3 Alice, 2 Bob
	votingClient := client.NewClient(identityURL, castingURL, pubKey)
	for i := 0; i < 5; i++ {
		voterID := fmt.Sprintf("voter-%03d", i+1)
		reg, err := votingClient.Register(voterID, electionID)
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		var votes []int64
		if i < 3 {
			votes = []int64{1, 0}
		} else {
			votes = []int64{0, 1}
		}
		if _, err := votingClient.Cast(electionID, reg.Credential, votes); err != nil {
			t.Fatalf("cast: %v", err)
		}
	}
	t.Log("cast 5 ballots (real: Alice=3, Bob=2)")

	// 4. Freeze
	fab.Submit(t, "FreezeElection", electionID)
	t.Log("election frozen")

	// 5. Fetch encrypted tally (real ciphertexts)
	encRaw := fab.Evaluate(t, "GetEncryptedTally", electionID)
	var encTally struct {
		Candidates []string `json:"candidates"`
		C1s        []string `json:"c1s"`
		C2s        []string `json:"c2s"`
	}
	if err := json.Unmarshal(encRaw, &encTally); err != nil {
		t.Fatalf("parse encrypted tally: %v", err)
	}

	// 6. Compute REAL sums: Alice=3, Bob=2
	realSums := []int64{3, 2}

	// 7. Reconstruct the private key to generate real proofs
	ceremonyDir := CeremonyDir(runID)
	sharePaths, err := findShares(ceremonyDir)
	if err != nil {
		t.Fatalf("find shares: %v", err)
	}
	if len(sharePaths) < 3 {
		t.Fatalf("need 3 shares, found %d", len(sharePaths))
	}
	sharePaths = sharePaths[:3]
	passphrases := []string{
		"dev-keyholder-1", "dev-keyholder-2", "dev-keyholder-3",
	}
	_, outLen, _ := keyceremony.LoadShare(sharePaths[0], passphrases[0])
	sk, err := keyceremony.ReconstructPrivateKey(sharePaths, passphrases, outLen)
	if err != nil {
		t.Fatalf("reconstruct key: %v", err)
	}
	sk.PublicKey = *pubKey

	// 8. Generate CORRECT ZK proofs for the real sums
	realProofs := make([]*zkproofs.DecryptionProof, 2)
	for i := 0; i < 2; i++ {
		c1, _ := new(big.Int).SetString(encTally.C1s[i], 10)
		c2, _ := new(big.Int).SetString(encTally.C2s[i], 10)
		m := new(big.Int).Exp(pubKey.G, big.NewInt(realSums[i]), pubKey.P)
		proof, err := zkproofs.GenerateDecryptionProof(
			c1, c2, m, pubKey.Y, pubKey.P, pubKey.G, sk.X,
		)
		if err != nil {
			t.Fatalf("generate proof for %s: %v", encTally.Candidates[i], err)
		}
		realProofs[i] = proof
	}
	t.Log("generated correct ZK proofs for real sums (Alice=3, Bob=2)")

	// 9. Publish WRONG sums with the REAL proofs attached
	tamperedSums := []int64{5, 0} // Alice=5, Bob=0 (tampered)
	type chaincodeCandidateResult struct {
		Candidate string `json:"candidate"`
		Sum       int64  `json:"sum"`
		Proof     string `json:"proof"`
	}
	var results []chaincodeCandidateResult
	for i, c := range encTally.Candidates {
		proofJSON, _ := json.Marshal(realProofs[i])
		results = append(results, chaincodeCandidateResult{
			Candidate: c,
			Sum:       tamperedSums[i],
			Proof:     string(proofJSON),
		})
	}
	resultsJSON, _ := json.Marshal(results)

	if _, err := fab.contract.SubmitTransaction(
		"PublishTally", electionID, string(resultsJSON), "5",
	); err != nil {
		t.Fatalf("PublishTally (tampered): %v", err)
	}
	t.Logf("published tampered tally: Alice=5, Bob=0 (real was 3, 2)")
	t.Log("")
	t.Logf("TAMPERED_ELECTION_ID=%s", electionID)
	t.Logf("CEREMONY_DIR=%s", runID)

	// For the demo recording, we can stop here — the verifier will fail.
	if os.Getenv("SKIP_TALLY") == "1" {
		return
	}

	// Sanity check: the verifier should fail
	t.Log("")
	t.Log("Running verifier against tampered election (should fail)...")
	// (The actual verifier run happens in the demo script, not here.)
}

// findShares returns the share files in a ceremony directory.
func findShares(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var shares []string
	for _, e := range entries {
		if e.Name() != "ceremony.json" && e.Name() != "publickey.json" {
			shares = append(shares, dir+"/"+e.Name())
		}
	}
	return shares, nil
}

// Ensure elgamal is imported (used via pubKey type from ProvisionCeremony).
var _ = elgamal.PublicKey{}
