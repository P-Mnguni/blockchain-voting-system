package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// BallotContract is the Fabric chaincode contract for the voting system.
type BallotContract struct {
	contractapi.Contract
}

/*
getTxTimestamp returns the transaction timestamp, or the current time if
running under the mock stub
*/
func getTxTimestamp(ctx contractapi.TransactionContextInterface) (int64, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil || ts == nil {
		return time.Now().Unix(), nil
	}
	return ts.Seconds, nil
}

// ELECTION LIFECYCLE

// RegisterElection creates a new election. Called once per electionID.
func (c *BallotContract) RegisterElection(
	ctx contractapi.TransactionContextInterface,
	electionID string,
	name string,
	candidatesJSON string,
	homomorphicPubKey string,
) error {
	if electionID == "" || name == "" || homomorphicPubKey == "" {
		return errors.New("electionID, name, and homomorphicPubKey are required")
	}

	var candidates []string
	if err := json.Unmarshal([]byte(candidatesJSON), &candidates); err != nil {
		return fmt.Errorf("candidatesJSON must be a JSON array of strings: %w", err)
	}
	if len(candidates) < 2 {
		return errors.New("an election must have at least two candidates")
	}

	existing, err := ctx.GetStub().GetState(electionKey(electionID))
	if err != nil {
		return fmt.Errorf("failed to read ledger: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("election %q already exists", electionID)
	}

	now, err := getTxTimestamp(ctx)
	if err != nil {
		return err
	}

	election := Election{
		ElectionID:        electionID,
		Name:              name,
		Phase:             PhaseRegistration,
		Candidates:        candidates,
		HomomorphicPubKey: homomorphicPubKey,
		RegisteredAt:      now,
	}

	return ctx.GetStub().PutState(electionKey(electionID), mustMarshal(election))
}

// StartVoting transitions an election from REGISTRATION to VOTING
func (c *BallotContract) StartVoting(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) error {
	election, err := c.getElection(ctx, electionID)
	if err != nil {
		return err
	}
	if election.Phase != PhaseRegistration {
		return fmt.Errorf("cannot start voting: election is in phase %s", election.Phase)
	}
	election.Phase = PhaseVoting
	return ctx.GetStub().PutState(electionKey(electionID), mustMarshal(election))
}

// FreezeElection transitions an election from VOTING to FROZEN
func (c *BallotContract) FreezeElection(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) error {
	election, err := c.getElection(ctx, electionID)
	if err != nil {
		return err
	}
	if election.Phase == PhaseFrozen || election.Phase == PhaseTallied {
		return nil
	}
	if election.Phase != PhaseVoting {
		return fmt.Errorf("cannot freeze: election is in phase %s", election.Phase)
	}

	now, _ := getTxTimestamp(ctx)
	election.Phase = PhaseFrozen
	election.FrozenAt = now
	return ctx.GetStub().PutState(electionKey(electionID), mustMarshal(election))
}

// getElection reads and unmarshals an election from the ledger
func (c *BallotContract) getElection(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) (*Election, error) {
	data, err := ctx.GetStub().GetState(electionKey(electionID))
	if err != nil {
		return nil, fmt.Errorf("failed to read election: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("election %q not found", electionID)
	}
	var e Election
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("corrupt election record: %w", err)
	}
	return &e, nil
}

// BALLOT OPERATIONS

/*
SubmitBallot casts a vote. Requirements:
  - election must exist and be in VOTING phase
  - the token hash must not already be spent in this election
  - ballotID must not collide with an existing ballot
  - encryptedVotesJSON must be a JSON array with exactly one entry per candidate
*/
func (c *BallotContract) SubmitBallot(
	ctx contractapi.TransactionContextInterface,
	electionID string,
	ballotID string,
	encryptedVotesJSON string,
	tokenHash string,
) error {
	if electionID == "" || ballotID == "" || tokenHash == "" {
		return errors.New("electionID, ballotID, and tokenHash are required")
	}
	if len(encryptedVotesJSON) == 0 {
		return errors.New("encryptedVotes is required")
	}

	election, err := c.getElection(ctx, electionID)
	if err != nil {
		return err
	}
	if election.Phase != PhaseVoting {
		return fmt.Errorf("election %q is not accepting ballots (phase %s)", electionID, election.Phase)
	}

	// Parse and validate per-candidate ciphertexts
	var votes []string
	if err := json.Unmarshal([]byte(encryptedVotesJSON), &votes); err != nil {
		return fmt.Errorf("encryptedVotesJSON must be a JSON array of strings: %w", err)
	}
	if len(votes) != len(election.Candidates) {
		return fmt.Errorf("expected %d encrypted votes, got %d", len(election.Candidates), len(votes))
	}
	for i, v := range votes {
		if v == "" {
			return fmt.Errorf("encrypted vote %d is empty", i)
		}
	}

	// Reject duplicate ballot IDs
	existing, err := ctx.GetStub().GetState(ballotKey(electionID, ballotID))
	if err != nil {
		return fmt.Errorf("failed to read ledger: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("ballot %q already exists", ballotID)
	}

	// Reject spent tokens
	spent, err := ctx.GetStub().GetState(tokenKey(electionID, tokenHash))
	if err != nil {
		return fmt.Errorf("failed to check token: %w", err)
	}
	if spent != nil {
		return errors.New("token has already been used in this election")
	}

	now, _ := getTxTimestamp(ctx)
	ballot := Ballot{
		BallotID:       ballotID,
		ElectionID:     electionID,
		EncryptedVotes: votes,
		TokenHash:      tokenHash,
		CastAt:         now,
		ChannelID:      ctx.GetStub().GetChannelID(),
	}

	if err := ctx.GetStub().PutState(ballotKey(electionID, ballotID), mustMarshal(ballot)); err != nil {
		return err
	}

	// Mark token as spent
	token := TokenRecord{
		TokenHash:  tokenHash,
		ElectionID: electionID,
		SpentAt:    now,
	}
	if err := ctx.GetStub().PutState(tokenKey(electionID, tokenHash), mustMarshal(token)); err != nil {
		return err
	}

	eventPayload := mustMarshal(map[string]string{
		"electionID": electionID,
		"ballotID":   ballotID,
	})
	if err := ctx.GetStub().SetEvent("BallotCast", eventPayload); err != nil {
		return err
	}

	return nil
}

// GetBallot returns a single ballet by ID
func (c *BallotContract) GetBallot(
	ctx contractapi.TransactionContextInterface,
	electionID string,
	ballotID string,
) (*Ballot, error) {
	data, err := ctx.GetStub().GetState(ballotKey(electionID, ballotID))
	if err != nil {
		return nil, fmt.Errorf("failed to read ballot: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("ballot %q not found in election %q", ballotID, electionID)
	}
	var b Ballot
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("corrupt ballot record: %w", err)
	}
	return &b, nil
}

// HasTokenBeenSpent returns true if a token hash has already been used
func (c *BallotContract) HasTokenBeenSpent(
	ctx contractapi.TransactionContextInterface,
	electionID string,
	tokenHash string,
) (bool, error) {
	data, err := ctx.GetStub().GetState(tokenKey(electionID, tokenHash))
	if err != nil {
		return false, fmt.Errorf("failed to check token: %w", err)
	}
	return data != nil, nil
}

// TALLY

/*
GetEncryptedTally computes the homomorphic sum of all ballots in an election.
Returns one (C1, C2) pair per candidate.
Results are cached under encryptedTallyKey(electionID) once computed.
*/
func (c *BallotContract) GetEncryptedTally(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) (*PerCandidateTally, error) {
	election, err := c.getElection(ctx, electionID)
	if err != nil {
		return nil, err
	}

	// Return cached if present
	if cached, err := ctx.GetStub().GetState(encryptedTallyKey(electionID)); err == nil && cached != nil {
		var t PerCandidateTally
		if err := json.Unmarshal(cached, &t); err == nil {
			return &t, nil
		}
	}

	numCandidates := len(election.Candidates)
	sumC1 := make([]*big.Int, numCandidates)
	sumC2 := make([]*big.Int, numCandidates)
	for i := 0; i < numCandidates; i++ {
		sumC1[i] = big.NewInt(1)
		sumC2[i] = big.NewInt(1)
	}

	iterator, err := ctx.GetStub().GetStateByRange(
		ballotKey(electionID, ""),
		ballotKey(electionID, "~"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate ballots: %w", err)
	}
	defer iterator.Close()

	var count int64
	for iterator.HasNext() {
		entry, err := iterator.Next()
		if err != nil {
			return nil, fmt.Errorf("iterator error: %w", err)
		}
		var b Ballot
		if err := json.Unmarshal(entry.Value, &b); err != nil {
			return nil, fmt.Errorf("corrupt ballot record: %w", err)
		}
		if len(b.EncryptedVotes) != numCandidates {
			return nil, fmt.Errorf("ballot %s has %d votes, expected %d", b.BallotID, len(b.EncryptedVotes), numCandidates)
		}
		for i, v := range b.EncryptedVotes {
			c1, c2, err := parseCiphertext([]byte(v))
			if err != nil {
				return nil, fmt.Errorf("ballot %s candidate %d: %w", b.BallotID, i, err)
			}
			sumC1[i].Mul(sumC1[i], c1)
			sumC2[i].Mul(sumC2[i], c2)
		}
		count++
	}

	c1s := make([]string, numCandidates)
	c2s := make([]string, numCandidates)
	for i := 0; i < numCandidates; i++ {
		c1s[i] = sumC1[i].String()
		c2s[i] = sumC2[i].String()
	}

	tally := PerCandidateTally{
		ElectionID:  electionID,
		Candidates:  election.Candidates,
		C1s:         c1s,
		C2s:         c2s,
		BallotCount: count,
	}
	now, _ := getTxTimestamp(ctx)
	tally.ComputedAt = now

	if err := ctx.GetStub().PutState(encryptedTallyKey(electionID), mustMarshal(tally)); err != nil {
		return nil, err
	}
	return &tally, nil
}

/*
	PublishTally writes the decrypted per-candidate sums and ZK proofs to the
	ledger. Only valid when the election is FROZEN (polls closed, results not
	yet published). Once published, the election transitions to TALLIED.
*/

// The caller must provide one proof per candidate.
func (c *BallotContract) PublishTally(
	ctx contractapi.TransactionContextInterface,
	electionID string,
	candidateResultsJSON string,
	totalBallots int64,
) error {
	if electionID == "" || candidateResultsJSON == "" {
		return errors.New("electionID and candidateResultsJSON are required")
	}
	if totalBallots < 0 {
		return errors.New("totalBallots must be non-negative")
	}

	election, err := c.getElection(ctx, electionID)
	if err != nil {
		return err
	}

	// Only publishable when FROZEN. Prevents publishing on a live election.
	if election.Phase != PhaseFrozen {
		return fmt.Errorf("cannot publish tally: election is in phase %s (must be FROZEN)", election.Phase)
	}

	// Parse the candidate results.
	var results []CandidateResult
	if err := json.Unmarshal([]byte(candidateResultsJSON), &results); err != nil {
		return fmt.Errorf("candidateResultsJSON must be a JSON array of CandidateResult: %w", err)
	}

	// Every candidate must have exactly one result.
	if len(results) != len(election.Candidates) {
		return fmt.Errorf("expected %d candidate results, got %d", len(election.Candidates), len(results))
	}

	// Each result's candidate name must match the election's declared candidate.
	candidateSet := make(map[string]bool, len(election.Candidates))
	for _, name := range election.Candidates {
		candidateSet[name] = true
	}
	for _, r := range results {
		if !candidateSet[r.Candidate] {
			return fmt.Errorf("unknown candidate %q", r.Candidate)
		}
		if r.Proof == "" {
			return fmt.Errorf("candidate %q has no proof", r.Candidate)
		}
	}

	now, _ := getTxTimestamp(ctx)
	tally := &TallyResult{
		ElectionID:       electionID,
		CandidateResults: results,
		TotalBallots:     totalBallots,
		PublishedAt:      now,
	}

	// Store the tally result.
	if err := ctx.GetStub().PutState(tallyResultKey(electionID), mustMarshal(tally)); err != nil {
		return err
	}

	// Transition the election to TALLIED and embed the result.
	election.Phase = PhaseTallied
	// election.TallyResult = tally
	if err := ctx.GetStub().PutState(electionKey(electionID), mustMarshal(election)); err != nil {
		return err
	}

	// Emit event for verifier portal / public listeners.
	eventPayload := mustMarshal(map[string]interface{}{
		"electionID":   electionID,
		"totalBallots": totalBallots,
		"publishedAt":  now,
	})
	if err := ctx.GetStub().SetEvent("TallyPublished", eventPayload); err != nil {
		return err
	}

	return nil
}

/*
GetTally returns the published tally result for an election, or an error if
the tally hasn't been published yet.
*/
func (c *BallotContract) GetTally(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) (*TallyResult, error) {
	data, err := ctx.GetStub().GetState(tallyResultKey(electionID))
	if err != nil {
		return nil, fmt.Errorf("failed to read tally: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("tally for election %q not published yet", electionID)
	}
	var t TallyResult
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("corrupt tally record: %w", err)
	}
	return &t, nil
}

// GetAllBallots returns all ballots for an election.
func (c *BallotContract) GetAllBallots(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) ([]Ballot, error) {
	// Verify election exists
	if _, err := c.getElection(ctx, electionID); err != nil {
		return nil, err
	}

	iterator, err := ctx.GetStub().GetStateByRange(
		ballotKey(electionID, ""),
		ballotKey(electionID, "~"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate ballots: %w", err)
	}
	defer iterator.Close()

	var ballots []Ballot
	for iterator.HasNext() {
		entry, err := iterator.Next()
		if err != nil {
			return nil, fmt.Errorf("iterator error: %w", err)
		}
		var b Ballot
		if err := json.Unmarshal(entry.Value, &b); err != nil {
			return nil, fmt.Errorf("corrupt ballot record: %w", err)
		}
		ballots = append(ballots, b)
	}
	return ballots, nil
}

/*
parseCiphertext extracts (C1, C2) from a JSON-serialized ciphertext
*/
func parseCiphertext(blob []byte) (*big.Int, *big.Int, error) {
	var raw struct {
		C1 string `json:"C1"`
		C2 string `json:"C2"`
	}
	if err := json.Unmarshal(blob, &raw); err != nil {
		return nil, nil, fmt.Errorf("invalid ciphertext JSON: %w", err)
	}
	c1, ok := new(big.Int).SetString(raw.C1, 10)
	if !ok {
		return nil, nil, errors.New("invalid C1")
	}
	c2, ok := new(big.Int).SetString(raw.C2, 10)
	if !ok {
		return nil, nil, errors.New("invalid C2")
	}
	return c1, c2, nil
}

// UTILITY

// HashToken is a convenience for producing the SHA-256 hex hash of a raw credential
func HashToken(rawToken []byte) string {
	sum := sha256.Sum256(rawToken)
	return hex.EncodeToString(sum[:])
}

// GetElection returns an election by ID.
func (c *BallotContract) GetElection(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) (*Election, error) {
	return c.getElection(ctx, electionID)
}
