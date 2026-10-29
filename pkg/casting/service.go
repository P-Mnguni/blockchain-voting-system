package casting

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/crypto/sha256util"
	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/identity"
)

// Service handles ballot casting.
type Service struct {
	Fabric         fabricclient.Client
	Issuer         *blind.BlindSignatureIssuer
	ExpirySkewtime time.Duration
}

// CastRequest is the input to Cast.
type CastRequest struct {
	ElectionID      string // which election to cast into
	EncryptedBallot string
	Credential      string
}

// Receipt proves inclusion on the ledger without revealing the vote.
type Receipt struct {
	BallotID    string `json:"ballotId"`
	TxID        string `json:"txId"`
	CommittedAt int64  `json:"committedAt"`
}

// Cast validates the credential and submits the ballot to Fabric.
func (s *Service) Cast(req CastRequest) (*Receipt, error) {
	if s.Fabric == nil || s.Issuer == nil {
		return nil, errors.New("service not fully initialized")
	}
	if req.ElectionID == "" {
		return nil, errors.New("electionID is required")
	}
	if req.EncryptedBallot == "" || req.Credential == "" {
		return nil, errors.New("encryptedBallot and credential are required")
	}

	// Decode the credential.
	cred, err := identity.DecodeCredential(req.Credential)
	if err != nil {
		return nil, err
	}

	// Verify signature.
	if !cred.Verify(s.Issuer) {
		return nil, errors.New("invalid credential signature")
	}

	// Check credential's election binding matches the request.
	expectedHash := sha256util.Sum([]byte(req.ElectionID))
	if cred.ElectionIDHash != expectedHash {
		return nil, errors.New("credential is for a different election")
	}

	// Check expiry.
	skew := s.ExpirySkewtime
	if skew == 0 {
		skew = 5 * time.Minute
	}
	if time.Now().After(cred.Expiry.Add(skew)) {
		return nil, fmt.Errorf("credential expired at %s", cred.Expiry.Format(time.RFC3339))
	}

	tokenHash := cred.Hash()

	// Fast-path: check if token is already spent.
	spentRaw, err := s.Fabric.Evaluate("HasTokenBeenSpent", req.ElectionID, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("check token: %w", err)
	}
	if isTrue(spentRaw) {
		return nil, errors.New("credential already used for this election")
	}

	ballotID := sha256util.Hex([]byte(tokenHash + ":" + time.Now().UTC().Format(time.RFC3339Nano)))
	committedAt := time.Now().UTC().Unix()

	if _, err := s.Fabric.Submit("SubmitBallot",
		req.ElectionID, ballotID, req.EncryptedBallot, tokenHash,
	); err != nil {
		return nil, fmt.Errorf("submit ballot: %w", err)
	}

	return &Receipt{
		BallotID:    ballotID,
		CommittedAt: committedAt,
	}, nil
}

// isTrue parses "true" from the chaincode's JSON-encoded bool response.
func isTrue(b []byte) bool {
	var v bool
	if err := json.Unmarshal(b, &v); err == nil {
		return v
	}
	return string(b) == "true"
}
