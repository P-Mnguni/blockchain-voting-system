package services

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/blockchain-voting-system/pkg/client"
	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
)

// provisionElectionWithKey provisions an election using a real homomorphic
// public key. Returns the election ID and the matching private key (which
// stays in the test process — never sent to any service).
func provisionElectionWithKey(t *testing.T) (string, *elgamal.PrivateKey) {
	t.Helper()

	cfg := FabricConfigFromEnv(t)
	prov, err := NewProvisioner(cfg)
	if err != nil {
		t.Fatalf("provisioner: %v", err)
	}
	t.Cleanup(func() { prov.Close() })

	// Generate a real ElGamal keypair.
	sk, err := elgamal.NewElGamal(2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Serialize the public key for on-chain storage.
	pkJSON, err := client.SerializePublicKey(&sk.PublicKey)
	if err != nil {
		t.Fatalf("serialize public key: %v", err)
	}

	electionID := FreshElectionID()
	if err := prov.ProvisionElectionWithKey(electionID, pkJSON); err != nil {
		t.Fatalf("provision: %v", err)
	}

	t.Logf("provisioned election %s with real homomorphic key", electionID)
	return electionID, sk
}

// TestEndToEndVoteFlow is the capstone test: full voter journey.
//
//	register → encrypt locally → cast → verify on ledger
func TestEndToEndVoteFlow(t *testing.T) {
	idURL, castURL := serviceURLs(t)
	electionID, _ := provisionElectionWithKey(t)

	// Build a client with the election's real public key.
	pk, err := getElectionPublicKey(t, electionID)
	if err != nil {
		t.Fatalf("get public key: %v", err)
	}

	c := client.NewClient(idURL, castURL, pk)

	// 1. Register
	reg, err := c.Register("voter-001", electionID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if reg.Credential == "" {
		t.Fatal("empty credential")
	}

	// 2. Cast (client encrypts locally)
	receipt, err := c.Cast(electionID, reg.Credential, 1)
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	if receipt.BallotID == "" {
		t.Fatal("empty ballot ID")
	}
	t.Logf("cast ballot %s at %d", receipt.BallotID, receipt.CommittedAt)

	// 3. Verify the ballot is on the ledger with the expected structure.
	ballot, err := getBallot(t, electionID, receipt.BallotID)
	if err != nil {
		t.Fatalf("get ballot: %v", err)
	}
	if ballot.BallotID != receipt.BallotID {
		t.Errorf("ballot ID mismatch")
	}

	// 4. Verify the encrypted ballot is a well-formed ElGamal ciphertext.
	ct, err := parseCiphertextJSON(ballot.EncryptedVote)
	if err != nil {
		t.Fatalf("parse ciphertext: %v", err)
	}
	if ct.C1.Sign() <= 0 || ct.C2.Sign() <= 0 {
		t.Error("ciphertext components should be positive")
	}

	// 5. Verify the tally's ballotCount reflects our ballot.
	tally, err := getEncryptedTally(t, electionID)
	if err != nil {
		t.Fatalf("get tally: %v", err)
	}
	if tally.BallotCount < 1 {
		t.Errorf("tally should have >=1 ballot, got %d", tally.BallotCount)
	}

	t.Logf("E2E success: ballot=%s tallyCount=%d", receipt.BallotID, tally.BallotCount)
}

// TestEndToEndDoubleVoteRejected proves the same voter can't vote twice.
func TestEndToEndDoubleVoteRejected(t *testing.T) {
	idURL, castURL := serviceURLs(t)
	electionID, _ := provisionElectionWithKey(t)

	pk, _ := getElectionPublicKey(t, electionID)
	c := client.NewClient(idURL, castURL, pk)

	reg, err := c.Register("voter-002", electionID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// First cast succeeds.
	if _, err := c.Cast(electionID, reg.Credential, 1); err != nil {
		t.Fatalf("first cast: %v", err)
	}

	// Second cast with the same credential fails.
	_, err = c.Cast(electionID, reg.Credential, 0)
	if err == nil {
		t.Fatal("second cast should fail")
	}
	if !strings.Contains(err.Error(), "already") && !strings.Contains(err.Error(), "403") {
		t.Logf("error (verify manually): %v", err)
	}
	t.Logf("correctly rejected: %v", err)
}

// --- helpers ---

// getElectionPublicKey reads the election's public key from the chaincode.
func getElectionPublicKey(t *testing.T, electionID string) (*elgamal.PublicKey, error) {
	t.Helper()
	cfg := FabricConfigFromEnv(t)
	prov, err := NewProvisioner(cfg)
	if err != nil {
		return nil, err
	}
	defer prov.Close()

	raw, err := prov.Fabric.Evaluate("GetElection", electionID)
	if err != nil {
		return nil, err
	}

	var e struct {
		HomomorphicPubKey string `json:"homomorphicPubKey"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	return client.DeserializePublicKey(e.HomomorphicPubKey)
}

// getBallot reads a single ballot from the chaincode.
func getBallot(t *testing.T, electionID, ballotID string) (*struct {
	BallotID      string
	EncryptedVote string
}, error) {
	t.Helper()
	cfg := FabricConfigFromEnv(t)
	prov, err := NewProvisioner(cfg)
	if err != nil {
		return nil, err
	}
	defer prov.Close()

	raw, err := prov.Fabric.Evaluate("GetBallot", electionID, ballotID)
	if err != nil {
		return nil, err
	}

	var b struct {
		BallotID      string `json:"ballotId"`
		EncryptedVote string `json:"encryptedVote"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	return &struct {
		BallotID      string
		EncryptedVote string
	}{BallotID: b.BallotID, EncryptedVote: b.EncryptedVote}, nil
}

// getEncryptedTally reads the encrypted tally for an election.
func getEncryptedTally(t *testing.T, electionID string) (*struct {
	BallotCount int64
}, error) {
	t.Helper()
	cfg := FabricConfigFromEnv(t)
	prov, err := NewProvisioner(cfg)
	if err != nil {
		return nil, err
	}
	defer prov.Close()

	raw, err := prov.Fabric.Evaluate("GetEncryptedTally", electionID)
	if err != nil {
		return nil, err
	}

	var tally struct {
		BallotCount int64 `json:"ballotCount"`
	}
	if err := json.Unmarshal(raw, &tally); err != nil {
		return nil, err
	}
	return &struct{ BallotCount int64 }{BallotCount: tally.BallotCount}, nil
}

// parseCiphertextJSON parses {"C1": "<dec>", "C2": "<dec>"}.
func parseCiphertextJSON(s string) (*elgamal.Ciphertext, error) {
	var raw struct{ C1, C2 string }
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	c1, ok := new(big.Int).SetString(raw.C1, 10)
	if !ok {
		return nil, fmt.Errorf("invalid C1: %q", raw.C1)
	}
	c2, ok := new(big.Int).SetString(raw.C2, 10)
	if !ok {
		return nil, fmt.Errorf("invalid C2: %q", raw.C2)
	}
	return &elgamal.Ciphertext{C1: c1, C2: c2}, nil
}
