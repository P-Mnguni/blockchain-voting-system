package main

import (
	"encoding/json"
	"fmt"
)

// ElectionPhase represents the lifecycle state of an election.
type ElectionPhase string

const (
	PhaseRegistration ElectionPhase = "REGISTRATION"
	PhaseVoting       ElectionPhase = "VOTING"
	PhaseFrozen       ElectionPhase = "FROZEN"
	PhaseTallied      ElectionPhase = "TALLIED"
)

// Election holds the static configuration of an election and its current phase.
type Election struct {
	ElectionID        string        `json:"electionId"`
	Name              string        `json:"name"`
	Phase             ElectionPhase `json:"phase"`
	Candidates        []string      `json:"candidates"`
	HomomorphicPubKey string        `json:"homomorphicPubKey"`
	RegisteredAt      int64         `json:"registeredAt"`
	FrozenAt          int64         `json:"frozenAt.omitempty"`
}

// Ballot is an encrypted vote as stored on the ledger.
//
// EncryptedVotes is one ciphertext per candidate, in the same order as
// Election.Candidates. A vote for candidate i is encrypted as Enc(1) at
// position i and Enc(0) at all other positions.
type Ballot struct {
	BallotID       string   `json:"ballotId"`
	ElectionID     string   `json:"electionId"`
	EncryptedVotes []string `json:"encryptedVotes"`
	TokenHash      string   `json:"tokenHash"`
	CastAt         int64    `json:"castAt"`
	ChannelID      string   `json:"channelId"`
}

// TokenRecord marks a token hash as spent.Existence = spent.
type TokenRecord struct {
	TokenHash  string `json:"tokenHash"`
	ElectionID string `json:"electionId"`
	SpentAt    int64  `json:"spentAt"`
}

// PerCandidateTally is the homomorphic sum per candidate for an election.
// C1s and C2s are the two components of each candidate's accumulated ElGamal
// ciphertext, stored as decimal strings.
type PerCandidateTally struct {
	ElectionID  string   `json:"electionId"`
	Candidates  []string `json:"candidates"` // same order as Election.Candidates
	C1s         []string `json:"c1s"`        // one per candidate
	C2s         []string `json:"c2s"`        // one per candidate
	BallotCount int64    `json:"ballotCount"`
	ComputedAt  int64    `json:"computedAt"`
}

// HELPERS FOR COMPOSITE KEYS

// electionKey returns the ledger key for an election
func electionKey(electionID string) string {
	return fmt.Sprintf("ELECTION_%s", electionID)
}

// ballotKey returns the ledger key for a ballot
func ballotKey(electionID, ballotID string) string {
	return fmt.Sprintf("Ballot_%s_%s", electionID, ballotID)
}

// tokenKey returns the ledger key for a spent-token record
func tokenKey(electionID, tokenHash string) string {
	return fmt.Sprintf("TOKEN_%s_%s", electionID, tokenHash)
}

// encryptedTallyKey returns the ledger key for an encrypted tally
func encryptedTallyKey(electionID string) string {
	return fmt.Sprintf("TALLYENC_%s", electionID)
}

// mustMarshal panics on marshal error
func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal failed: %v", err))
	}
	return b
}

// CandidateResult is one candidate's decrypted total plus the ZK proof that
// decryption was correct.
type CandidateResult struct {
	Candidate string `json:"candidate"`
	Sum       int64  `json:"sum"`
	Proof     string `json:"proof"` // JSON-serialized zkproofs.DecryptionProof
}

// TallyResult is the published, decrypted tally for an election.
// Written to the ledger after the threshold decryption ceremony.
type TallyResult struct {
	ElectionID       string            `json:"electionId"`
	CandidateResults []CandidateResult `json:"candidateResults"`
	TotalBallots     int64             `json:"totalBallots"`
	PublishedAt      int64             `json:"publishedAt"`
}

// tallyResultKey returns the ledger key for a published tally result.
func tallyResultKey(electionID string) string {
	return fmt.Sprintf("TALLYRESULT_%s", electionID)
}
