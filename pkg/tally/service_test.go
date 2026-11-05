package tally

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/keyceremony"
)

// setupCeremony generates a ceremony and casts N encrypted ballots via a mock
// fabric client, then returns everything the tally service needs.
func setupCeremony(t *testing.T) (cfg Config, fab *fabricclient.MockClient) {
	t.Helper()

	// 1. Create the ceremony
	dir := t.TempDir()
	result, err := keyceremony.RunTrustedDealer(keyceremony.Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024,
		PassphrasePrefix: "test-pass-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	// 2. Create encrypted ballots: 10 for candidate A, 5 for candidate B
	const numBallots = 15
	type encBallot struct {
		C1s, C2s []string
	}
	var ballots []encBallot
	for i := 0; i < numBallots; i++ {
		var votes []int64
		if i < 10 {
			votes = []int64{1, 0}
		} else {
			votes = []int64{0, 1}
		}

		c1s := make([]string, 2)
		c2s := make([]string, 2)
		for j, v := range votes {
			ct, err := result.PublicKey.EncryptVote(v)
			if err != nil {
				t.Fatalf("EncryptVote failed: %v", err)
			}
			c1s[j] = ct.C1.String()
			c2s[j] = ct.C2.String()
		}
		ballots = append(ballots, encBallot{C1s: c1s, C2s: c2s})
	}

	// 3. Compute expected homomorphic sum (just for reference — we'll verify via decryption)
	sumC1 := []*big.Int{big.NewInt(1), big.NewInt(1)}
	sumC2 := []*big.Int{big.NewInt(1), big.NewInt(1)}
	for _, b := range ballots {
		for i := 0; i < 2; i++ {
			c1, _ := new(big.Int).SetString(b.C1s[i], 10)
			c2, _ := new(big.Int).SetString(b.C2s[i], 10)
			sumC1[i].Mul(sumC1[i], c1)
			sumC2[i].Mul(sumC2[i], c2)
		}
	}

	// 4. Mock fabric client
	fab = &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			switch fn {
			case "GetEncryptedTally":
				tally := map[string]interface{}{
					"electionId":  args[0],
					"candidates":  []string{"Alice", "Bob"},
					"c1s":         []string{sumC1[0].String(), sumC1[1].String()},
					"c2s":         []string{sumC2[0].String(), sumC2[1].String()},
					"ballotCount": numBallots,
				}
				return json.Marshal(tally)
			default:
				t.Fatalf("unexpected Evaluate: %s", fn)
				return nil, nil
			}
		},
		SubmitFn: func(fn string, args ...string) ([]byte, error) {
			if fn != "PublishTally" {
				t.Fatalf("unexpected Submit: %s", fn)
			}
			return nil, nil
		},
	}

	cfg = Config{
		ElectionID:       "test-election",
		CeremonyDir:      dir,
		PassphrasePrefix: "test-pass-",
		Fabric:           fab,
	}
	return cfg, fab
}

func TestServiceRunSuccess(t *testing.T) {
	cfg, fab := setupCeremony(t)
	svc := &Service{}

	result, err := svc.Run(cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.ElectionID != "test-election" {
		t.Errorf("electionID mismatch: %s", result.ElectionID)
	}
	if result.TotalBallots != 15 {
		t.Errorf("totalBallots: got %d, want 15", result.TotalBallots)
	}
	if len(result.CandidateResults) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(result.CandidateResults))
	}

	// Verify sums: Alice should have 10, Bob should have 5
	sums := map[string]int64{}
	for _, r := range result.CandidateResults {
		sums[r.Candidate] = r.Sum
	}
	if sums["Alice"] != 10 {
		t.Errorf("Alice: got %d, want 10", sums["Alice"])
	}
	if sums["Bob"] != 5 {
		t.Errorf("Bob: got %d, want 5", sums["Bob"])
	}

	// Verify every result has a proof
	for _, r := range result.CandidateResults {
		if r.Proof == nil {
			t.Errorf("candidate %q has no proof", r.Candidate)
		}
	}

	// Verify PublishTally was called
	if len(fab.SubmitCalls) != 1 || fab.SubmitCalls[0].Fn != "PublishTally" {
		t.Errorf("PublishTally not called")
	}
}

func TestServiceRunRejectsEmptyElection(t *testing.T) {
	cfg, fab := setupCeremony(t)
	fab.EvalFn = func(fn string, args ...string) ([]byte, error) {
		// Return empty tally
		tally := map[string]interface{}{
			"electionId":  args[0],
			"candidates":  []string{"Alice", "Bob"},
			"c1s":         []string{"1", "1"},
			"c2s":         []string{"1", "1"},
			"ballotCount": 0,
		}
		return json.Marshal(tally)
	}

	svc := &Service{}
	_, err := svc.Run(cfg)
	if err == nil {
		t.Fatal("Run should fail on empty election")
	}
}

func TestServiceRunInvalidConfig(t *testing.T) {
	svc := &Service{}

	cases := []struct {
		name string
		cfg  Config
	}{
		{"missing electionID", Config{CeremonyDir: "/tmp", PassphrasePrefix: "p"}},
		{"missing ceremonyDir", Config{ElectionID: "e", PassphrasePrefix: "p"}},
		{"missing passphrasePrefix", Config{ElectionID: "e", CeremonyDir: "/tmp"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.Run(c.cfg); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestParseCiphertext(t *testing.T) {
	ct, err := parseCiphertext("123", "456")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ct.C1.Cmp(big.NewInt(123)) != 0 {
		t.Errorf("C1 mismatch")
	}
	if ct.C2.Cmp(big.NewInt(456)) != 0 {
		t.Errorf("C2 mismatch")
	}

	_, err = parseCiphertext("invalid", "456")
	if err == nil {
		t.Error("invalid C1 should fail")
	}
}

// Ensure elgamal is used somewhere so the import isn't dropped
var _ = elgamal.Ciphertext{}
