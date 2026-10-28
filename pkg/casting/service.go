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
	Issuer         *blind.BlindSignatureIssuer // public-only; used to verify credentials
	ElectionID     string
	ExpirySkewtime time.Duration // tolerated clock skew, defaults to 5 min
}

// CastRequest is the input to Cast.
type CastRequest struct {
	EncryptedBallot string // JSON-encoded ciphertext (client-side encrypted)
	Credential      string // credential.Encode() output
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
	if s.ElectionID == "" {
		return nil, errors.New("electionID not configured")
	}
	if req.EncryptedBallot == "" || req.Credential == "" {
		return nil, errors.New("encryptedBallot and credential are required")
	}

	// 1. Decode the credential.
	cred, err := identity.DecodeCredential(req.Credential)
	if err != nil {
		return nil, err
	}

	// 2. Verify the signature against the issuer's public key.
	if !cred.Verify(s.Issuer) {
		return nil, errors.New("invalid credential signature")
	}

	// 3. Check election binding.
	expectedElectionHash := sha256util.Sum([]byte(s.ElectionID))
	if cred.ElectionIDHash != expectedElectionHash {
		return nil, fmt.Errorf("credential is for a different election")
	}

	// 4. Check expiry (with configurable skew).
	skew := s.ExpirySkewtime
	if skew == 0 {
		skew = 5 * time.Minute
	}
	if time.Now().After(cred.Expiry.Add(skew)) {
		return nil, fmt.Errorf("credential expired at %s", cred.Expiry.Format(time.RFC3339))
	}

	// 5. Compute the token hash the chaincode will use for double-spend detection.
	tokenHash := cred.Hash()

	// 6. Check whether the token has already been spent (fast path).
	spentRaw, err := s.Fabric.Evaluate("HasTokenBeenSpent", s.ElectionID, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("check token: %w", err)
	}
	if isTrue(spentRaw) {
		return nil, errors.New("credential already used for this election")
	}

	// 7. Generate a unique ballot ID (the chaincode expects a unique per-election ID).
	ballotID := sha256util.Hex([]byte(tokenHash + ":" + time.Now().UTC().Format(time.RFC3339Nano)))

	// 8. Submit to Fabric.
	committedAt := time.Now().UTC().Unix()
	txBytes, err := s.Fabric.Submit("SubmitBallot",
		s.ElectionID,
		ballotID,
		req.EncryptedBallot,
		tokenHash,
	)
	if err != nil {
		return nil, fmt.Errorf("submit ballot: %w", err)
	}

	// The chaincode doesn't return the txID. If the caller needs it for block
	// explorer lookup, extend the chaincode to emit it. For Milestone 3 we
	// synthesize a receipt txID from the response (which is empty on success).
	_ = txBytes

	return &Receipt{
		BallotID:    ballotID,
		TxID:        "", // populated by the HTTP layer if it can access the gateway's txID
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
