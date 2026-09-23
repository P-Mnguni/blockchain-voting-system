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
*/
func (c *BallotContract) SubmitBallot(
	ctx contractapi.TransactionContextInterface,
	electionID string,
	ballotID string,
	encryptedVoteJSON string,
	tokenHash string,
) error {
	if electionID == "" || ballotID == "" || tokenHash == "" {
		return errors.New("electionID, ballotID, and tokenHash are required")
	}
	if len(encryptedVoteJSON) == 0 {
		return errors.New("encryptedVote is required")
	}

	election, err := c.getElection(ctx, electionID)
	if err != nil {
		return err
	}
	if election.Phase != PhaseVoting {
		return fmt.Errorf("election %q is not accepting ballots (phase %s)", electionID, election.Phase)
	}

	existing, err := ctx.GetStub().GetState(ballotKey(electionID, ballotID))
	if err != nil {
		return fmt.Errorf("failed to read ledger: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("ballot %q already exists", ballotID)
	}

	spent, err := ctx.GetStub().GetState(tokenKey(electionID, tokenHash))
	if err != nil {
		return fmt.Errorf("failed to check token: %w", err)
	}
	if spent != nil {
		return errors.New("token has already been used in this election")
	}

	now, _ := getTxTimestamp(ctx)
	ballot := Ballot{
		BallotID:      ballotID,
		ElectionID:    electionID,
		EncryptedVote: []byte(encryptedVoteJSON),
		TokenHash:     tokenHash,
		CastAt:        now,
		ChannelID:     ctx.GetStub().GetChannelID(),
	}

	if err := ctx.GetStub().PutState(ballotKey(electionID, ballotID), mustMarshal(ballot)); err != nil {
		return err
	}

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

// GetEncryptedTally computes the homomorphic sum of all ballots in an election.
func (c *BallotContract) GetEncryptedTally(
	ctx contractapi.TransactionContextInterface,
	electionID string,
) (*EncryptedTally, error) {
	if _, err := c.getElection(ctx, electionID); err != nil {
		return nil, err
	}

	if cached, err := ctx.GetStub().GetState(tallyKey(electionID)); err == nil && cached != nil {
		var t EncryptedTally
		if err := json.Unmarshal(cached, &t); err == nil {
			return &t, nil
		}
	}

	iterator, err := ctx.GetStub().GetStateByRange(
		ballotKey(electionID, ""),
		ballotKey(electionID, "~"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate ballots: %w", err)
	}
	defer iterator.Close()

	sumC1 := big.NewInt(1)
	sumC2 := big.NewInt(1)
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

		c1, c2, err := parseCiphertext(b.EncryptedVote)
		if err != nil {
			return nil, fmt.Errorf("ballot %s: %w", b.BallotID, err)
		}
		sumC1.Mul(sumC1, c1)
		sumC2.Mul(sumC2, c2)
		count++
	}

	tally := EncryptedTally{
		ElectionID:  electionID,
		C1:          sumC1.String(),
		C2:          sumC2.String(),
		BallotCount: count,
	}
	now, _ := getTxTimestamp(ctx)
	tally.ComputedAt = now

	if err := ctx.GetStub().PutState(tallyKey(electionID), mustMarshal(tally)); err != nil {
		return nil, err
	}
	return &tally, nil
}

/*
parseCiphertext extracts (C1, C2) from a JSON-serialized ciphertext
Expected to look like {"C1": <bigint-string>, "C2": <bigint-string>}
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
