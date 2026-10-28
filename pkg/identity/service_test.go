package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/blind"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	issuer, err := blind.NewBlindSignatureIssuer(2048)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	return &Service{
		Issuer:        issuer,
		EID:           NewMockEID(),
		Voters:        NewMemoryVoterStore(),
		CredentialTTL: time.Hour,
	}
}

func TestRegisterSuccess(t *testing.T) {
	s := newTestService(t)
	resp, err := s.Register("voter-001", "election-2025")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if resp.Credential == nil {
		t.Fatal("credential is nil")
	}
	if len(resp.Credential.Signature) == 0 {
		t.Error("signature is empty")
	}
	if resp.Credential.Expiry.Before(time.Now()) {
		t.Error("expiry is in the past")
	}
}

func TestRegisterRejectsBadEID(t *testing.T) {
	s := newTestService(t)
	_, err := s.Register("voter-999", "election-2025")
	if err == nil {
		t.Fatal("expected eID error")
	}
	var eidErr *EIDError
	if !errors.As(err, &eidErr) {
		t.Errorf("expected *EIDError, got %T", err)
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Register("voter-001", "election-2025"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	_, err := s.Register("voter-001", "election-2025")
	if err == nil {
		t.Fatal("duplicate register should fail")
	}
	var are *AlreadyRegisteredError
	if !errors.As(err, &are) {
		t.Errorf("expected *AlreadyRegisteredError, got %T", err)
	}
}

func TestRegisterAllowsSameVoterDifferentElections(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Register("voter-001", "e1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := s.Register("voter-001", "e2"); err != nil {
		t.Errorf("different election should work: %v", err)
	}
}

func TestRegisterRejectsEmpty(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Register("", "e1"); err == nil {
		t.Error("empty voterID should fail")
	}
	if _, err := s.Register("voter-001", ""); err == nil {
		t.Error("empty electionID should fail")
	}
}

func TestCredentialRoundTrip(t *testing.T) {
	s := newTestService(t)
	resp, _ := s.Register("voter-001", "e1")
	enc := resp.Credential.Encode()

	dec, err := DecodeCredential(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.Hash() != resp.Credential.Hash() {
		t.Errorf("hash mismatch")
	}
	if !dec.Verify(s.Issuer) {
		t.Error("decoded credential failed verification")
	}
}

func TestCredentialRejectsTamperedPayload(t *testing.T) {
	s := newTestService(t)
	resp, _ := s.Register("voter-001", "e1")
	enc := resp.Credential.Encode()

	// Flip a bit in the payload
	mutated := []byte(enc)
	mutated[5] ^= 0x01
	dec, err := DecodeCredential(string(mutated))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.Verify(s.Issuer) {
		t.Error("tampered credential verified — signature check is broken")
	}
}

func TestCredentialHashIsStable(t *testing.T) {
	s := newTestService(t)
	resp, _ := s.Register("voter-001", "e1")
	h1 := resp.Credential.Hash()
	h2 := resp.Credential.Hash()
	if h1 != h2 {
		t.Error("hash is not deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("hash length %d, want 64", len(h1))
	}
}

func TestServiceNotInitialized(t *testing.T) {
	s := &Service{}
	_, err := s.Register("voter-001", "e1")
	if err == nil || !strings.Contains(err.Error(), "not fully initialized") {
		t.Errorf("expected initialization error, got %v", err)
	}
}
