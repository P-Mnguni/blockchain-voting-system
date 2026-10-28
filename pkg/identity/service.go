package identity

import (
	"errors"
	"fmt"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/crypto/sha256util"
)

// Service issues voting credentials.
type Service struct {
	Issuer *blind.BlindSignatureIssuer
	EID    EIDProvider
	Voters VoterStore
	// CredentialTTL is how long an issued credential is valid. Defaults to 24h.
	CredentialTTL time.Duration
}

// RegisterResponse is what the service returns to a caller.
type RegisterResponse struct {
	Credential *Credential
}

// Register verifies the voter and issues a credential for the election.
func (s *Service) Register(voterID, electionID string) (*RegisterResponse, error) {
	if voterID == "" || electionID == "" {
		return nil, errors.New("voterID and electionID are required")
	}
	if s.Issuer == nil || s.EID == nil || s.Voters == nil {
		return nil, errors.New("service not fully initialized")
	}

	ttl := s.CredentialTTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	// 1. Verify the voter's eID.
	if err := s.EID.Verify(voterID); err != nil {
		return nil, err
	}

	// 2. Mark registered (atomic — returns error if already registered).
	if err := s.Voters.MarkRegistered(voterID, electionID); err != nil {
		return nil, err
	}

	// 3. Build the credential payload.
	cred := &Credential{
		VoterIDHash:    sha256Bytes(voterID),
		ElectionIDHash: sha256Bytes(electionID),
		Expiry:         time.Now().Add(ttl).UTC(),
	}

	// 4. Blind-sign the payload. Note: the issuer sees only the blinded payload,
	//    not the raw voterID — this is what makes the credential unlinkable.
	sig, err := s.Issuer.IssueCredential(cred.Payload())
	if err != nil {
		return nil, fmt.Errorf("sign credential: %w", err)
	}
	cred.Signature = sig

	return &RegisterResponse{Credential: cred}, nil
}

// sha256Bytes returns the SHA-256 of s as a [32]byte.
func sha256Bytes(s string) [32]byte {
	return sha256util.Sum([]byte(s))
}
