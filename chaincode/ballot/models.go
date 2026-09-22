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

// Ballot is an encrypted vote as stored on the ledger
type Ballot struct {
	BallotID      string `json:"ballotId"`
	ElectionID    string `json:"electionId"`
	EncryptedVote []byte `json:"encryptedVote"`
	TokenHash     string `json:"tokenHash"`
	CastAt        int64  `json:"castAt"`
	ChannelID     string `json:"channelId"`
}

// TokenRecord marks a token hash as spent.Existence = spent.
type TokenRecord struct {
	TokenHash  string `json:"tokenHash"`
	ElectionID string `json:"electionId"`
	SpentAt    int64  `json:"spentAt"`
}

// EncryptedTally is the homomorphic sum of all ballots in an election
type EncryptedTally struct {
	ElectionID  string `json:"electionId"`
	C1          string `json:"c1"`
	C2          string `json:"c2"`
	BallotCount int64  `json:"ballotCount"`
	ComputedAt  int64  `json:"computedAt"`
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

// tallyKey returns the ledger key for an encrypted tally
func tallyKey(electionID string) string {
	return fmt.Sprintf("TALLEY_%s", electionID)
}

// mustMarshal panics on marshal error
func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal failed: %v", err))
	}
	return b
}
