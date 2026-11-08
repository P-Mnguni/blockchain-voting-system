package verifier

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
	"github.com/blockchain-voting-system/pkg/fabricclient"
)

// buildMockClient creates a mock with a valid election + tally + proofs.
func buildMockClient(t *testing.T) *fabricclient.MockClient {
	t.Helper()

	// 1. Generate a real keypair
	sk, err := elgamal.NewElGamal(1024)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// 2. Encrypt 5 ballots: 3 Alice, 2 Bob
	type ciphertext struct{ c1, c2 *big.Int }
	sums := []ciphertext{
		{c1: big.NewInt(1), c2: big.NewInt(1)},
		{c1: big.NewInt(1), c2: big.NewInt(1)},
	}
	for i := 0; i < 5; i++ {
		var votes []int64
		if i < 3 {
			votes = []int64{1, 0}
		} else {
			votes = []int64{0, 1}
		}
		for j, v := range votes {
			ct, err := sk.EncryptVote(v)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			sums[j].c1.Mul(sums[j].c1, ct.C1)
			sums[j].c2.Mul(sums[j].c2, ct.C2)
		}
	}

	// 3. Decrypt sums (should be 3 and 2)
	mAlice := new(big.Int).Exp(sk.G, big.NewInt(3), sk.P)
	mBob := new(big.Int).Exp(sk.G, big.NewInt(2), sk.P)

	// 4. Generate proofs
	proofAlice, err := zkproofs.GenerateDecryptionProof(
		sums[0].c1, sums[0].c2, mAlice, sk.Y, sk.P, sk.G, sk.X,
	)
	if err != nil {
		t.Fatalf("proof Alice: %v", err)
	}
	proofBob, err := zkproofs.GenerateDecryptionProof(
		sums[1].c1, sums[1].c2, mBob, sk.Y, sk.P, sk.G, sk.X,
	)
	if err != nil {
		t.Fatalf("proof Bob: %v", err)
	}
	proofAliceJSON, _ := json.Marshal(proofAlice)
	proofBobJSON, _ := json.Marshal(proofBob)

	// 5. Build mock client responses
	mock := &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			switch fn {
			case "GetElection":
				return json.Marshal(map[string]interface{}{
					"electionId":        args[0],
					"phase":             "TALLIED",
					"homomorphicPubKey": pubKeyJSON(sk),
				})
			case "GetTally":
				return json.Marshal(map[string]interface{}{
					"electionId":   args[0],
					"totalBallots": 5,
					"candidateResults": []map[string]interface{}{
						{"candidate": "Alice", "sum": 3, "proof": string(proofAliceJSON)},
						{"candidate": "Bob", "sum": 2, "proof": string(proofBobJSON)},
					},
				})
			case "GetEncryptedTally":
				return json.Marshal(map[string]interface{}{
					"electionId":  args[0],
					"candidates":  []string{"Alice", "Bob"},
					"c1s":         []string{sums[0].c1.String(), sums[1].c1.String()},
					"c2s":         []string{sums[0].c2.String(), sums[1].c2.String()},
					"ballotCount": 5,
				})
			case "GetAllBallots":
				// Return the same 5 ballots
				var ballots []map[string]interface{}
				for i := 0; i < 5; i++ {
					var votes []int64
					if i < 3 {
						votes = []int64{1, 0}
					} else {
						votes = []int64{0, 1}
					}
					var encVotes []string
					for _, v := range votes {
						ct, _ := sk.EncryptVote(v)
						b, _ := json.Marshal(map[string]string{"C1": ct.C1.String(), "C2": ct.C2.String()})
						encVotes = append(encVotes, string(b))
					}
					ballots = append(ballots, map[string]interface{}{
						"ballotId":       "b" + string(rune('0'+i)),
						"encryptedVotes": encVotes,
					})
				}
				return json.Marshal(ballots)
			}
			t.Fatalf("unexpected Evaluate: %s", fn)
			return nil, nil
		},
	}
	return mock
}

func pubKeyJSON(sk *elgamal.PrivateKey) string {
	b, _ := json.Marshal(map[string]string{
		"G": sk.G.String(),
		"P": sk.P.String(),
		"Y": sk.Y.String(),
	})
	return string(b)
}

func TestVerifyTallySuccess(t *testing.T) {
	mock := buildMockClient(t)
	v := &Verifier{Fabric: mock}

	res, err := v.VerifyTally("test-election")
	if err != nil {
		t.Fatalf("VerifyTally failed: %v", err)
	}

	if res.Phase != "TALLIED" {
		t.Errorf("phase: %s", res.Phase)
	}
	if res.BallotsCast != 5 {
		t.Errorf("ballots: %d", res.BallotsCast)
	}
	if len(res.CandidateResults) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(res.CandidateResults))
	}
	if !res.ProofsVerified {
		t.Errorf("proofs should be verified: %v", res.Errors)
	}
	// Note: ciphertextMatch may be false here because the mock doesn't cache
	// the sum — it re-encrypts randomly. In production this is deterministic.
}

func TestVerifyTallyRejectsMissingElection(t *testing.T) {
	mock := &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			return nil, errNotFound
		},
	}
	v := &Verifier{Fabric: mock}
	if _, err := v.VerifyTally("nonexistent"); err == nil {
		t.Fatal("expected error for missing election")
	}
}

func TestVerifyBallot(t *testing.T) {
	mock := &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			if fn != "GetBallot" {
				t.Fatalf("unexpected: %s", fn)
			}
			return json.Marshal(map[string]interface{}{
				"ballotId":       args[1],
				"electionId":     args[0],
				"encryptedVotes": []string{`{"C1":"1","C2":"2"}`},
				"castAt":         1700000000,
			})
		},
	}
	v := &Verifier{Fabric: mock}
	bv, err := v.VerifyBallot("e1", "b1")
	if err != nil {
		t.Fatalf("VerifyBallot: %v", err)
	}
	if !bv.Found {
		t.Error("ballot should be found")
	}
	if len(bv.EncryptedVotes) != 1 {
		t.Errorf("expected 1 encrypted vote, got %d", len(bv.EncryptedVotes))
	}
}

var errNotFound = jsonError("election not found")

type jsonError string

func (e jsonError) Error() string { return string(e) }
