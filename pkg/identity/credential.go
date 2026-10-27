package identity

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/crypto/sha256util"
)

// Credential is a self-verifying voting credential.
//
//	credential = base64(payload) + "." + hex(signature)
//
// where payload = voterIDHash(32) || electionIDHash(32) || expiry(8)
//
// The credential does not reveal the voterID (only its hash), so a leaked
// credential cannot be used to deanonymize the voter.
type Credential struct {
	VoterIDHash    [32]byte
	ElectionIDHash [32]byte
	Expiry         time.Time
	Signature      []byte
}

// Payload returns the raw payload bytes (voterIDHash || electionIDHash || expiry).
func (c *Credential) Payload() []byte {
	buf := make([]byte, 32+32+8)
	copy(buf[:32], c.VoterIDHash[:])
	copy(buf[32:64], c.ElectionIDHash[:])
	binary.BigEndian.PutUint64(buf[64:], uint64(c.Expiry.Unix()))
	return buf
}

// Encode renders the credential as a compact string suitable for HTTP transport.
func (c *Credential) Encode() string {
	payload := base64.RawURLEncoding.EncodeToString(c.Payload())
	sig := hex.EncodeToString(c.Signature)
	return payload + "." + sig
}

// DecodeCredential parses a credential string. Does NOT verify the signature —
// the caller must do that via Verify.
func DecodeCredential(s string) (*Credential, error) {
	dot := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 || dot == len(s)-1 {
		return nil, fmt.Errorf("credential: malformed")
	}

	payload, err := base64.RawURLEncoding.DecodeString(s[:dot])
	if err != nil {
		return nil, fmt.Errorf("credential: bad payload encoding: %w", err)
	}
	if len(payload) != 32+32+8 {
		return nil, fmt.Errorf("credential: wrong payload length %d", len(payload))
	}

	sig, err := hex.DecodeString(s[dot+1:])
	if err != nil {
		return nil, fmt.Errorf("credential: bad signature encoding: %w", err)
	}

	c := &Credential{Signature: sig}
	copy(c.VoterIDHash[:], payload[:32])
	copy(c.ElectionIDHash[:], payload[32:64])
	c.Expiry = time.Unix(int64(binary.BigEndian.Uint64(payload[64:])), 0).UTC()
	return c, nil
}

// Verify checks the signature against the issuer's public key.
func (c *Credential) Verify(verifier *blind.BlindSignatureIssuer) bool {
	return verifier.VerifyCredential(c.Payload(), c.Signature)
}

// Hash returns the SHA-256 of the payload, suitable as the on-chain token hash.
func (c *Credential) Hash() string {
	return sha256util.Hex(c.Payload())
}
