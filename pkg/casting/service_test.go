package casting

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/identity"
)

// newTestSetup returns a casting service wired to a mock fabric and a fresh issuer.
func newTestSetup(t *testing.T) (*Service, *blind.BlindSignatureIssuer) {
	t.Helper()
	issuer, err := blind.NewBlindSignatureIssuer(2048)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}

	mock := &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			if fn != "HasTokenBeenSpent" {
				t.Fatalf("unexpected Evaluate: %s", fn)
			}
			return []byte("false"), nil
		},
		SubmitFn: func(fn string, args ...string) ([]byte, error) {
			if fn != "SubmitBallot" {
				t.Fatalf("unexpected Submit: %s", fn)
			}
			return nil, nil
		},
	}

	return &Service{
		Fabric: mock,
		Issuer: issuer,
	}, issuer
}

// makeCredential issues a valid credential for the given election.
func makeCredential(t *testing.T, issuer *blind.BlindSignatureIssuer, electionID, voterID string) string {
	t.Helper()
	svc := &identity.Service{
		Issuer:        issuer,
		EID:           identity.NewMockEID(),
		Voters:        identity.NewMemoryVoterStore(),
		CredentialTTL: time.Hour,
	}
	resp, err := svc.Register(voterID, electionID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return resp.Credential.Encode()
}

func TestCastSuccess(t *testing.T) {
	s, issuer := newTestSetup(t)
	cred := makeCredential(t, issuer, "election-2025", "voter-001")

	receipt, err := s.Cast(CastRequest{
		ElectionID:      "election-2025",
		EncryptedBallot: `{"C1":"123","C2":"456"}`,
		Credential:      cred,
	})
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}
	if receipt.BallotID == "" {
		t.Error("ballotID is empty")
	}
	if receipt.CommittedAt == 0 {
		t.Error("committedAt is zero")
	}
}

func TestCastRejectsMalformedCredential(t *testing.T) {
	s, _ := newTestSetup(t)
	_, err := s.Cast(CastRequest{
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      "garbage",
	})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestCastRejectsTamperedCredential(t *testing.T) {
	s, issuer := newTestSetup(t)
	cred := makeCredential(t, issuer, "election-2025", "voter-001")

	// Flip a bit in the payload
	b := []byte(cred)
	b[5] ^= 0x01

	_, err := s.Cast(CastRequest{
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      string(b),
	})
	if err == nil {
		t.Fatal("tampered credential should fail")
	}
}

func TestCastRejectsWrongElection(t *testing.T) {
	s, issuer := newTestSetup(t)
	cred := makeCredential(t, issuer, "election-OTHER", "voter-001")

	_, err := s.Cast(CastRequest{
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      cred,
	})
	if err == nil {
		t.Fatal("credential for a different election should fail")
	}
}

func TestCastRejectsExpiredCredential(t *testing.T) {
	s, issuer := newTestSetup(t)
	svc := &identity.Service{
		Issuer:        issuer,
		EID:           identity.NewMockEID(),
		Voters:        identity.NewMemoryVoterStore(),
		CredentialTTL: -time.Hour, // already expired
	}
	resp, _ := svc.Register("voter-001", "election-2025")

	_, err := s.Cast(CastRequest{
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      resp.Credential.Encode(),
	})
	if err == nil {
		t.Fatal("expired credential should fail")
	}
}

func TestCastRejectsAlreadySpent(t *testing.T) {
	s, issuer := newTestSetup(t)
	// Override mock to say "already spent"
	s.Fabric = &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			return []byte("true"), nil
		},
	}
	cred := makeCredential(t, issuer, "election-2025", "voter-001")

	_, err := s.Cast(CastRequest{
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      cred,
	})
	if err == nil {
		t.Fatal("already-spent credential should fail")
	}
}

func TestCastRejectsEmptyInput(t *testing.T) {
	s, _ := newTestSetup(t)
	if _, err := s.Cast(CastRequest{}); err == nil {
		t.Error("empty request should fail")
	}
	if _, err := s.Cast(CastRequest{EncryptedBallot: "x"}); err == nil {
		t.Error("missing credential should fail")
	}
}

func TestCastPropagatesFabricErrors(t *testing.T) {
	s, issuer := newTestSetup(t)
	sentinel := errors.New("fabric boom")
	s.Fabric = &fabricclient.MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			return []byte("false"), nil
		},
		SubmitFn: func(fn string, args ...string) ([]byte, error) {
			return nil, sentinel
		},
	}
	cred := makeCredential(t, issuer, "election-2025", "voter-001")

	_, err := s.Cast(CastRequest{
		ElectionID:      "election-2025",
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      cred,
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel, got %v", err)
	}
}

func TestCastRejectsMismatchedElection(t *testing.T) {
	s, issuer := newTestSetup(t)
	cred := makeCredential(t, issuer, "election-2025", "voter-001")

	// Credential is for election-2025, but request asks for election-OTHER
	_, err := s.Cast(CastRequest{
		ElectionID:      "election-OTHER",
		EncryptedBallot: `{"C1":"1","C2":"2"}`,
		Credential:      cred,
	})
	if err == nil || !strings.Contains(err.Error(), "different election") {
		t.Fatalf("expected election mismatch error, got %v", err)
	}
}
