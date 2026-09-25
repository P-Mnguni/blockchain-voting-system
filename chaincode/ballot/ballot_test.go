package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/blockchain-voting-system/chaincode/ballot/mock"
	"github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// testContext builds a contractapi.TransactionContextInterface wrapping our mock stub.
func testContext(t *testing.T) (contractapi.TransactionContextInterface, *mock.Stub) {
	t.Helper()
	stub := mock.NewStub()
	ctx := &testTxContext{stub: stub}
	return ctx, stub
}

// testTxContext is the minimal implementation of contractapi.TransactionContextInterface
// needed for our tests.
type testTxContext struct {
	stub shim.ChaincodeStubInterface
}

func (c *testTxContext) GetStub() shim.ChaincodeStubInterface {
	return c.stub
}
func (c *testTxContext) GetClientIdentity() cid.ClientIdentity {
	return nil
}

/*
func (c *testTxContext) GetLogger() *logger.FabricLogger {
	return nil
}
*/
// --- helper to make a fake encrypted vote ---

func makeEncryptedVote(c1, c2 string) string {
	b, _ := json.Marshal(map[string]string{"C1": c1, "C2": c2})
	return string(b)
}

// --- tests ---

func TestRegisterElection(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	err := c.RegisterElection(ctx, "election-2025", "General 2025", `["Alice","Bob"]`, "pubkey-pem")
	if err != nil {
		t.Fatalf("RegisterElection failed: %v", err)
	}

	// Fetch it back
	election, err := c.getElection(ctx, "election-2025")
	if err != nil {
		t.Fatalf("getElection failed: %v", err)
	}
	if election.Name != "General 2025" {
		t.Errorf("name mismatch: %s", election.Name)
	}
	if election.Phase != PhaseRegistration {
		t.Errorf("expected REGISTRATION, got %s", election.Phase)
	}
	if len(election.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(election.Candidates))
	}
}

func TestRegisterElectionDuplicateFails(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	if err := c.RegisterElection(ctx, "e1", "Election 1", `["A","B"]`, "pk"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := c.RegisterElection(ctx, "e1", "Election 1 again", `["A","B"]`, "pk")
	if err == nil {
		t.Error("duplicate RegisterElection should fail")
	}
}

func TestRegisterElectionInvalidCandidates(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	cases := map[string]string{
		"not json":         `hello`,
		"empty list":       `[]`,
		"single candidate": `["only"]`,
	}
	for name, cj := range cases {
		t.Run(name, func(t *testing.T) {
			err := c.RegisterElection(ctx, "e-"+name, "n", cj, "pk")
			if err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestStartVotingAndSubmitBallot(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	encVote := makeEncryptedVote("12345678", "87654321")
	must(t, c.SubmitBallot(ctx, "e1", "ballot-1", encVote, "token-hash-1"))

	ballot, err := c.GetBallot(ctx, "e1", "ballot-1")
	if err != nil {
		t.Fatalf("GetBallot failed: %v", err)
	}
	if ballot.BallotID != "ballot-1" {
		t.Errorf("ballot ID mismatch")
	}
	if string(ballot.EncryptedVote) != encVote {
		t.Errorf("encrypted vote mismatch")
	}
}

func TestSubmitBallotRejectedBeforeVotingStarts(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	// Phase is REGISTRATION, not VOTING
	err := c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVote("1", "2"), "t1")
	if err == nil {
		t.Error("SubmitBallot should fail before StartVoting")
	}
}

func TestSubmitBallotRejectedAfterFreeze(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.FreezeElection(ctx, "e1"))

	err := c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVote("1", "2"), "t1")
	if err == nil {
		t.Error("SubmitBallot should fail after FreezeElection")
	}
}

func TestDoubleSpendRejected(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	enc := makeEncryptedVote("1", "2")
	must(t, c.SubmitBallot(ctx, "e1", "b1", enc, "same-token"))

	// Same token, different ballot ID
	err := c.SubmitBallot(ctx, "e1", "b2", enc, "same-token")
	if err == nil {
		t.Error("double-spend with same token should fail")
	}
}

func TestDuplicateBallotIDRejected(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	enc := makeEncryptedVote("1", "2")
	must(t, c.SubmitBallot(ctx, "e1", "b1", enc, "token-1"))

	err := c.SubmitBallot(ctx, "e1", "b1", enc, "token-2")
	if err == nil {
		t.Error("duplicate ballot ID should fail")
	}
}

func TestHasTokenBeenSpent(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	spent, _ := c.HasTokenBeenSpent(ctx, "e1", "t1")
	if spent {
		t.Error("token should not be spent yet")
	}

	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVote("1", "2"), "t1"))

	spent, _ = c.HasTokenBeenSpent(ctx, "e1", "t1")
	if !spent {
		t.Error("token should be spent now")
	}
}

func TestFreezeIsIdempotent(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.FreezeElection(ctx, "e1"))

	// Second freeze should not error
	if err := c.FreezeElection(ctx, "e1"); err != nil {
		t.Errorf("idempotent freeze failed: %v", err)
	}
}

func TestGetEncryptedTally(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	// Three ballots with known C1/C2 values so we can verify the product
	// Homomorphic sum should be:
	//   C1 = 2 * 3 * 5 = 30
	//   C2 = 7 * 11 * 13 = 1001
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVote("2", "7"), "t1"))
	must(t, c.SubmitBallot(ctx, "e1", "b2", makeEncryptedVote("3", "11"), "t2"))
	must(t, c.SubmitBallot(ctx, "e1", "b3", makeEncryptedVote("5", "13"), "t3"))

	tally, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("GetEncryptedTally failed: %v", err)
	}
	if tally.BallotCount != 3 {
		t.Errorf("expected 3 ballots, got %d", tally.BallotCount)
	}

	c1, _ := new(big.Int).SetString(tally.C1, 10)
	c2, _ := new(big.Int).SetString(tally.C2, 10)

	if c1.Cmp(big.NewInt(30)) != 0 {
		t.Errorf("C1 mismatch: got %s, want 30", c1)
	}
	if c2.Cmp(big.NewInt(1001)) != 0 {
		t.Errorf("C2 mismatch: got %s, want 1001", c2)
	}
}

func TestGetEncryptedTallyCaches(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVote("2", "7"), "t1"))

	first, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("first tally failed: %v", err)
	}

	// Add another ballot — but since tally is cached, the second call
	// should return the same result (proves caching is working).
	must(t, c.SubmitBallot(ctx, "e1", "b2", makeEncryptedVote("3", "11"), "t2"))

	second, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("second tally failed: %v", err)
	}

	if first.C1 != second.C1 || first.C2 != second.C2 {
		t.Error("tally was not cached — second call recomputed")
	}
	if second.BallotCount != 1 {
		t.Errorf("cached tally should still show 1 ballot, got %d", second.BallotCount)
	}
}

func TestGetEncryptedTallyEmptyElection(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))

	tally, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("tally on empty election failed: %v", err)
	}
	if tally.BallotCount != 0 {
		t.Errorf("expected 0 ballots, got %d", tally.BallotCount)
	}
	// Sum should be identity (1, 1)
	if tally.C1 != "1" || tally.C2 != "1" {
		t.Errorf("empty tally should be (1,1), got (%s,%s)", tally.C1, tally.C2)
	}
}

func TestBallotCastEventEmitted(t *testing.T) {
	ctx, stub := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVote("1", "2"), "t1"))

	events := stub.Events()
	found := false
	for _, e := range events {
		if e.EventName == "BallotCast" {
			found = true
			if !strings.Contains(string(e.Payload), "b1") {
				t.Errorf("event payload missing ballotID: %s", e.Payload)
			}
		}
	}
	if !found {
		t.Error("BallotCast event was not emitted")
	}
}

// --- helpers ---

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// silence unused import warnings for fmt in case tests are trimmed
var _ = fmt.Sprintf
