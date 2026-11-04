package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
)

func newTestKeyPair(t *testing.T) (*elgamal.PrivateKey, *elgamal.PublicKey) {
	t.Helper()
	sk, err := elgamal.NewElGamal(2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return sk, &sk.PublicKey
}

func TestRegisterSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"credential": "cred-xyz",
			"expiry":     int64(9999999999),
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.URL, nil)
	resp, err := c.Register("voter-001", "election-1")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.Credential != "cred-xyz" {
		t.Errorf("credential mismatch: %s", resp.Credential)
	}
}

func TestRegisterHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not in registry"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.URL, nil)
	_, err := c.Register("voter-001", "election-1")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestCastEncryptsAndSubmits(t *testing.T) {
	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cast" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ballotId":    "ballot-123",
			"committedAt": int64(1700000000),
		})
	}))
	defer srv.Close()

	_, pubKey := newTestKeyPair(t)
	c := NewClient(srv.URL, srv.URL, pubKey)

	// Vote for candidate 0 (2 candidates)
	receipt, err := c.Cast("election-1", "cred-xyz", []int64{1, 0})
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	if receipt.BallotID != "ballot-123" {
		t.Errorf("ballot ID mismatch")
	}
	if capturedBody["encryptedBallot"] == "" {
		t.Fatal("encryptedBallot not sent")
	}
	// encryptedBallot should be a JSON array of 2 ciphertexts
	var votes []string
	if err := json.Unmarshal([]byte(capturedBody["encryptedBallot"]), &votes); err != nil {
		t.Fatalf("encryptedBallot is not a JSON array: %v (raw: %s)", err, capturedBody["encryptedBallot"])
	}
	if len(votes) != 2 {
		t.Errorf("expected 2 encrypted votes, got %d", len(votes))
	}
	for i, v := range votes {
		var ct struct{ C1, C2 string }
		if err := json.Unmarshal([]byte(v), &ct); err != nil {
			t.Fatalf("vote[%d] not a ciphertext: %v", i, err)
		}
		if ct.C1 == "" || ct.C2 == "" {
			t.Errorf("vote[%d] missing C1 or C2", i)
		}
	}
}

func TestCastRejectsInvalidVote(t *testing.T) {
	_, pubKey := newTestKeyPair(t)
	c := NewClient("http://unused", "http://unused", pubKey)

	// Empty array
	if _, err := c.Cast("e", "c", []int64{}); err == nil {
		t.Error("empty votes should be rejected")
	}
	// All zeros
	if _, err := c.Cast("e", "c", []int64{0, 0}); err == nil {
		t.Error("all-zero votes should be rejected")
	}
	// More than one 1
	if _, err := c.Cast("e", "c", []int64{1, 1}); err == nil {
		t.Error("multiple 1-votes should be rejected")
	}
	// Value 2
	if _, err := c.Cast("e", "c", []int64{2, 0}); err == nil {
		t.Error("vote=2 should be rejected")
	}
	// Negative
	if _, err := c.Cast("e", "c", []int64{-1, 1}); err == nil {
		t.Error("negative vote should be rejected")
	}
}

func TestCastRequiresPublicKey(t *testing.T) {
	c := NewClient("http://unused", "http://unused", nil)
	if _, err := c.Cast("e", "c", []int64{1, 0}); err == nil {
		t.Error("missing public key should fail")
	}
}

func TestPublicKeyRoundTrip(t *testing.T) {
	_, pubKey := newTestKeyPair(t)
	encoded, err := SerializePublicKey(pubKey)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	decoded, err := DeserializePublicKey(encoded)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if decoded.P.Cmp(pubKey.P) != 0 || decoded.G.Cmp(pubKey.G) != 0 || decoded.Y.Cmp(pubKey.Y) != 0 {
		t.Error("round-trip changed the key")
	}
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient("", "", nil)
	if err := c.Healthz(srv.URL); err != nil {
		t.Errorf("healthz: %v", err)
	}
}
