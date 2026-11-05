// Package tally orchestrates the decryption ceremony for a Fabric election.
// It fetches encrypted ballots, homomorphically sums them per candidate,
// reconstructs the threshold private key from keyholder shares, decrypts the
// sums, generates ZK proofs of correctness, and publishes the result back to
// the chaincode.
package tally

import (
	"encoding/json"
	"fmt"

	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
)

// CandidateResult is one candidate's decrypted total plus its ZK proof.
// This mirrors the on-chain CandidateResult struct, but with the proof as a
// structured field for easier verification on the verifier side.
type CandidateResult struct {
	Candidate string                    `json:"candidate"`
	Sum       int64                     `json:"sum"`
	Proof     *zkproofs.DecryptionProof `json:"proof"`
}

// Result is the full tally result for an election.
type Result struct {
	ElectionID       string            `json:"electionId"`
	CandidateResults []CandidateResult `json:"candidateResults"`
	TotalBallots     int64             `json:"totalBallots"`
}

// serializeCandidateResults converts the structured results into the JSON
// string format expected by the chaincode's PublishTally function.
// Each proof is serialized as a JSON string.
func serializeCandidateResults(results []CandidateResult) (string, error) {
	// Convert to the chaincode-compatible shape: Candidate, Sum, Proof (string)
	type chaincodeCandidateResult struct {
		Candidate string `json:"candidate"`
		Sum       int64  `json:"sum"`
		Proof     string `json:"proof"`
	}

	out := make([]chaincodeCandidateResult, len(results))
	for i, r := range results {
		if r.Proof == nil {
			return "", fmt.Errorf("candidate %q has no proof", r.Candidate)
		}
		proofJSON, err := json.Marshal(r.Proof)
		if err != nil {
			return "", fmt.Errorf("serialize proof for %q: %w", r.Candidate, err)
		}
		out[i] = chaincodeCandidateResult{
			Candidate: r.Candidate,
			Sum:       r.Sum,
			Proof:     string(proofJSON),
		}
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("serialize results: %w", err)
	}
	return string(raw), nil
}

// deserializeCandidateResults converts the chaincode's JSON string back to
// structured CandidateResult values.
func deserializeCandidateResults(s string) ([]CandidateResult, error) {
	type chaincodeCandidateResult struct {
		Candidate string `json:"candidate"`
		Sum       int64  `json:"sum"`
		Proof     string `json:"proof"`
	}

	var raw []chaincodeCandidateResult
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("parse candidate results: %w", err)
	}

	out := make([]CandidateResult, len(raw))
	for i, r := range raw {
		var proof zkproofs.DecryptionProof
		if err := json.Unmarshal([]byte(r.Proof), &proof); err != nil {
			return nil, fmt.Errorf("parse proof for %q: %w", r.Candidate, err)
		}
		out[i] = CandidateResult{
			Candidate: r.Candidate,
			Sum:       r.Sum,
			Proof:     &proof,
		}
	}
	return out, nil
}
