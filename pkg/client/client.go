// Package client is a Go client library for the identity and casting HTTP
// services. It also handles client-side ballot encryption, so plaintext votes
// never leave the caller.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
)

// Client talks to the identity and casting services.
type Client struct {
	IdentityURL string
	CastingURL  string
	HTTPClient  *http.Client

	// ElectionPubKey is the homomorphic public key used to encrypt ballots.
	// The client must obtain this from the election metadata before calling Cast.
	ElectionPubKey *elgamal.PublicKey
}

// NewClient returns a Client with sensible defaults.
func NewClient(identityURL, castingURL string, pubKey *elgamal.PublicKey) *Client {
	return &Client{
		IdentityURL:    identityURL,
		CastingURL:     castingURL,
		HTTPClient:     &http.Client{Timeout: 15 * time.Second},
		ElectionPubKey: pubKey,
	}
}

// RegisterResponse mirrors the identity service's POST /register response.
type RegisterResponse struct {
	Credential string `json:"credential"`
	Expiry     int64  `json:"expiry"`
}

// Receipt mirrors the casting service's POST /cast response.
type Receipt struct {
	BallotID    string `json:"ballotId"`
	TxID        string `json:"txId"`
	CommittedAt int64  `json:"committedAt"`
}

// Register requests a credential for the given voter and election.
func (c *Client) Register(voterID, electionID string) (*RegisterResponse, error) {
	if c.IdentityURL == "" {
		return nil, fmt.Errorf("identity URL not set")
	}
	if voterID == "" || electionID == "" {
		return nil, fmt.Errorf("voterID and electionID are required")
	}

	body := map[string]string{
		"voterId":    voterID,
		"electionId": electionID,
	}
	raw, code, err := c.postJSON(c.IdentityURL+"/register", body)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("register: HTTP %d: %s", code, raw)
	}

	var resp RegisterResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode register response: %w", err)
	}
	return &resp, nil
}

// Cast encrypts one vote per candidate (each 0 or 1), then submits to the
// casting service. votes[i] is the vote for election.Candidates[i].
//
// Exactly one entry should be 1 (for the chosen candidate) and the rest 0.
// The client validates this before sending.
func (c *Client) Cast(electionID, credential string, votes []int64) (*Receipt, error) {
	if c.CastingURL == "" {
		return nil, fmt.Errorf("casting URL not set")
	}
	if c.ElectionPubKey == nil {
		return nil, fmt.Errorf("election public key not set")
	}
	if electionID == "" || credential == "" {
		return nil, fmt.Errorf("electionID and credential are required")
	}
	if len(votes) == 0 {
		return nil, fmt.Errorf("votes array is empty")
	}
	// Validate: each is 0 or 1, exactly one is 1
	ones := 0
	for i, v := range votes {
		if v != 0 && v != 1 {
			return nil, fmt.Errorf("vote[%d] must be 0 or 1, got %d", i, v)
		}
		if v == 1 {
			ones++
		}
	}
	if ones != 1 {
		return nil, fmt.Errorf("exactly one vote must be 1, got %d", ones)
	}

	// Encrypt each candidate's vote
	encryptedVotes := make([]string, len(votes))
	for i, vote := range votes {
		ct, err := c.ElectionPubKey.EncryptVote(vote)
		if err != nil {
			return nil, fmt.Errorf("encrypt vote[%d]: %w", i, err)
		}
		b, err := marshalCiphertext(ct)
		if err != nil {
			return nil, fmt.Errorf("marshal vote[%d]: %w", i, err)
		}
		encryptedVotes[i] = b
	}

	// Serialize the array
	votesJSON, err := json.Marshal(encryptedVotes)
	if err != nil {
		return nil, fmt.Errorf("marshal votes array: %w", err)
	}

	// Submit
	body := map[string]string{
		"electionId":      electionID,
		"encryptedBallot": string(votesJSON),
		"credential":      credential,
	}
	raw, code, err := c.postJSON(c.CastingURL+"/cast", body)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("cast: HTTP %d: %s", code, raw)
	}

	var receipt Receipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return nil, fmt.Errorf("decode cast response: %w", err)
	}
	return &receipt, nil
}

// Healthz calls /healthz on the given service URL.
func (c *Client) Healthz(baseURL string) error {
	resp, err := c.HTTPClient.Get(baseURL + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: HTTP %d", resp.StatusCode)
	}
	return nil
}

// --- helpers ---

func (c *Client) postJSON(url string, body interface{}) ([]byte, int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encode request: %w", err)
	}
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, 0, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// marshalCiphertext serializes an ElGamal ciphertext as a JSON object with
// decimal-string fields. Uses a local struct rather than the elgamal.Ciphertext
// directly because big.Int marshals as a string of digits but we want explicit
// decimal strings for the chaincode.
func marshalCiphertext(ct *elgamal.Ciphertext) (string, error) {
	if ct == nil || ct.C1 == nil || ct.C2 == nil {
		return "", fmt.Errorf("ciphertext is nil")
	}
	b, err := json.Marshal(map[string]string{
		"C1": ct.C1.String(),
		"C2": ct.C2.String(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal ciphertext: %w", err)
	}
	return string(b), nil
}

// SerializePublicKey encodes a public key as JSON, for storing in the
// election's homomorphicPubKey field.
func SerializePublicKey(pk *elgamal.PublicKey) (string, error) {
	if pk == nil || pk.G == nil || pk.P == nil || pk.Y == nil {
		return "", fmt.Errorf("public key is nil")
	}
	b, err := json.Marshal(map[string]string{
		"G": pk.G.String(),
		"P": pk.P.String(),
		"Y": pk.Y.String(),
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DeserializePublicKey parses a public key previously encoded by SerializePublicKey.
func DeserializePublicKey(s string) (*elgamal.PublicKey, error) {
	var raw struct {
		G string `json:"G"`
		P string `json:"P"`
		Y string `json:"Y"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	g, ok := newBigInt(raw.G)
	if !ok {
		return nil, fmt.Errorf("invalid G")
	}
	p, ok := newBigInt(raw.P)
	if !ok {
		return nil, fmt.Errorf("invalid P")
	}
	y, ok := newBigInt(raw.Y)
	if !ok {
		return nil, fmt.Errorf("invalid Y")
	}

	return &elgamal.PublicKey{G: g, P: p, Y: y}, nil
}

// newBigInt parses a decimal string as a big.Int.
func newBigInt(s string) (*big.Int, bool) {
	if s == "" {
		return nil, false
	}
	v, ok := new(big.Int).SetString(s, 10)
	return v, ok
}
