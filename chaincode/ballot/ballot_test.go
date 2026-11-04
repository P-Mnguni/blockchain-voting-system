package main

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/blockchain-voting-system/chaincode/ballot/mock"
	"github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func testContext(t *testing.T) (contractapi.TransactionContextInterface, *mock.Stub) {
	t.Helper()
	stub := mock.NewStub()
	return &testTxContext{stub: stub}, stub
}

type testTxContext struct {
	stub shim.ChaincodeStubInterface
}

func (c *testTxContext) GetStub() shim.ChaincodeStubInterface  { return c.stub }
func (c *testTxContext) GetClientIdentity() cid.ClientIdentity { return nil }

// makeEncryptedVotes produces a JSON array of ciphertexts — one "C1,C2" pair
// per candidate, in election candidate order.
func makeEncryptedVotes(pairs ...string) string {
	var votes []string
	for _, p := range pairs {
		parts := strings.SplitN(p, ",", 2)
		b, _ := json.Marshal(map[string]string{"C1": parts[0], "C2": parts[1]})
		votes = append(votes, string(b))
	}
	out, _ := json.Marshal(votes)
	return string(out)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Election lifecycle ---

func TestRegisterElection(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "election-2025", "General 2025", `["Alice","Bob"]`, "pubkey-pem"))

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

	must(t, c.RegisterElection(ctx, "e1", "Election 1", `["A","B"]`, "pk"))
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

	encVotes := makeEncryptedVotes("10,20", "30,40")
	must(t, c.SubmitBallot(ctx, "e1", "ballot-1", encVotes, "token-hash-1"))

	ballot, err := c.GetBallot(ctx, "e1", "ballot-1")
	if err != nil {
		t.Fatalf("GetBallot failed: %v", err)
	}
	if ballot.BallotID != "ballot-1" {
		t.Errorf("ballot ID mismatch")
	}
	if len(ballot.EncryptedVotes) != 2 {
		t.Errorf("expected 2 encrypted votes, got %d", len(ballot.EncryptedVotes))
	}
}

func TestSubmitBallotRejectedBeforeVotingStarts(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	err := c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("1,2", "3,4"), "t1")
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

	err := c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("1,2", "3,4"), "t1")
	if err == nil {
		t.Error("SubmitBallot should fail after FreezeElection")
	}
}

func TestSubmitBallotRejectsWrongCandidateCount(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B","C"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	// Only 2 votes for 3 candidates
	err := c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("1,2", "3,4"), "t1")
	if err == nil {
		t.Fatal("SubmitBallot should reject wrong candidate count")
	}
	if !strings.Contains(err.Error(), "expected 3 encrypted votes") {
		t.Errorf("error should mention candidate count, got: %v", err)
	}
}

func TestSubmitBallotRejectsEmptyVote(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	// Empty string for the second vote
	encVotes := `[{"C1":"1","C2":"2"},""]`
	err := c.SubmitBallot(ctx, "e1", "b1", encVotes, "t1")
	if err == nil {
		t.Fatal("SubmitBallot should reject empty vote")
	}
}

func TestDoubleSpendRejected(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	enc := makeEncryptedVotes("1,2", "3,4")
	must(t, c.SubmitBallot(ctx, "e1", "b1", enc, "same-token"))

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

	enc := makeEncryptedVotes("1,2", "3,4")
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

	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("1,2", "3,4"), "t1"))

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

	if err := c.FreezeElection(ctx, "e1"); err != nil {
		t.Errorf("idempotent freeze failed: %v", err)
	}
}

// --- Per-candidate tally ---

func TestGetEncryptedTallyMultiCandidate(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	// 3 ballots: A=2, B=1
	// Ballot 1: A=1 (2,4), B=0 (3,9)
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("2,4", "3,9"), "t1"))
	// Ballot 2: A=1 (5,7), B=0 (11,13)
	must(t, c.SubmitBallot(ctx, "e1", "b2", makeEncryptedVotes("5,7", "11,13"), "t2"))
	// Ballot 3: A=0 (17,19), B=1 (23,29)
	must(t, c.SubmitBallot(ctx, "e1", "b3", makeEncryptedVotes("17,19", "23,29"), "t3"))

	tally, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("GetEncryptedTally failed: %v", err)
	}
	if tally.BallotCount != 3 {
		t.Errorf("ballot count: got %d, want 3", tally.BallotCount)
	}
	if len(tally.Candidates) != 2 {
		t.Errorf("candidates: got %d, want 2", len(tally.Candidates))
	}
	if len(tally.C1s) != 2 || len(tally.C2s) != 2 {
		t.Fatalf("expected 2 C1s and 2 C2s")
	}

	// C1_A = 2*5*17 = 170
	c1a, _ := new(big.Int).SetString(tally.C1s[0], 10)
	if c1a.Cmp(big.NewInt(170)) != 0 {
		t.Errorf("C1_A: got %s, want 170", c1a)
	}
	// C2_A = 4*7*19 = 532
	c2a, _ := new(big.Int).SetString(tally.C2s[0], 10)
	if c2a.Cmp(big.NewInt(532)) != 0 {
		t.Errorf("C2_A: got %s, want 532", c2a)
	}
	// C1_B = 3*11*23 = 759
	c1b, _ := new(big.Int).SetString(tally.C1s[1], 10)
	if c1b.Cmp(big.NewInt(759)) != 0 {
		t.Errorf("C1_B: got %s, want 759", c1b)
	}
	// C2_B = 9*13*29 = 3393
	c2b, _ := new(big.Int).SetString(tally.C2s[1], 10)
	if c2b.Cmp(big.NewInt(3393)) != 0 {
		t.Errorf("C2_B: got %s, want 3393", c2b)
	}
}

func TestGetEncryptedTallyCaches(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("2,7", "3,5"), "t1"))

	first, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("first tally failed: %v", err)
	}

	// Add another ballot — cached tally should NOT include it
	must(t, c.SubmitBallot(ctx, "e1", "b2", makeEncryptedVotes("3,11", "5,7"), "t2"))

	second, err := c.GetEncryptedTally(ctx, "e1")
	if err != nil {
		t.Fatalf("second tally failed: %v", err)
	}
	if first.C1s[0] != second.C1s[0] || first.C2s[0] != second.C2s[0] {
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
	// Both candidates should have identity ciphertext (1,1)
	for i := 0; i < 2; i++ {
		if tally.C1s[i] != "1" || tally.C2s[i] != "1" {
			t.Errorf("candidate %d empty tally should be (1,1), got (%s,%s)", i, tally.C1s[i], tally.C2s[i])
		}
	}
}

// --- GetAllBallots ---

func TestGetAllBallots(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("1,2", "3,4"), "t1"))
	must(t, c.SubmitBallot(ctx, "e1", "b2", makeEncryptedVotes("5,6", "7,8"), "t2"))

	ballots, err := c.GetAllBallots(ctx, "e1")
	if err != nil {
		t.Fatalf("GetAllBallots failed: %v", err)
	}
	if len(ballots) != 2 {
		t.Errorf("expected 2 ballots, got %d", len(ballots))
	}
	// Verify each has 2 encrypted votes
	for _, b := range ballots {
		if len(b.EncryptedVotes) != 2 {
			t.Errorf("ballot %s has %d votes, want 2", b.BallotID, len(b.EncryptedVotes))
		}
	}
}

func TestGetAllBallotsEmptyElection(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))

	ballots, err := c.GetAllBallots(ctx, "e1")
	if err != nil {
		t.Fatalf("GetAllBallots failed: %v", err)
	}
	if len(ballots) != 0 {
		t.Errorf("expected 0 ballots, got %d", len(ballots))
	}
}

func TestGetAllBallotsNonexistentElection(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	_, err := c.GetAllBallots(ctx, "nonexistent")
	if err == nil {
		t.Error("GetAllBallots should fail for nonexistent election")
	}
}

// --- PublishTally and GetTally ---

func TestPublishTallySuccess(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("2,7", "3,5"), "t1"))
	must(t, c.SubmitBallot(ctx, "e1", "b2", makeEncryptedVotes("3,11", "5,7"), "t2"))
	must(t, c.FreezeElection(ctx, "e1"))

	results := `[
		{"candidate":"A","sum":1,"proof":"fake-proof-a"},
		{"candidate":"B","sum":1,"proof":"fake-proof-b"}
	]`
	must(t, c.PublishTally(ctx, "e1", results, 2))

	tally, err := c.GetTally(ctx, "e1")
	if err != nil {
		t.Fatalf("GetTally failed: %v", err)
	}
	if tally.TotalBallots != 2 {
		t.Errorf("totalBallots: got %d, want 2", tally.TotalBallots)
	}
	if len(tally.CandidateResults) != 2 {
		t.Errorf("candidateResults: got %d, want 2", len(tally.CandidateResults))
	}

	// Election should be TALLIED
	election, _ := c.getElection(ctx, "e1")
	if election.Phase != PhaseTallied {
		t.Errorf("phase: got %s, want TALLIED", election.Phase)
	}
}

func TestPublishTallyRequiresFrozen(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))

	results := `[{"candidate":"A","sum":0,"proof":"p"},{"candidate":"B","sum":0,"proof":"p"}]`
	err := c.PublishTally(ctx, "e1", results, 0)
	if err == nil {
		t.Fatal("PublishTally should fail when election is not FROZEN")
	}
}

func TestPublishTallyRejectsMismatchedCandidates(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.FreezeElection(ctx, "e1"))

	results := `[{"candidate":"A","sum":1,"proof":"p"}]`
	err := c.PublishTally(ctx, "e1", results, 1)
	if err == nil {
		t.Fatal("PublishTally should fail with wrong candidate count")
	}
}

func TestPublishTallyRejectsUnknownCandidate(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.FreezeElection(ctx, "e1"))

	results := `[{"candidate":"A","sum":1,"proof":"p"},{"candidate":"C","sum":1,"proof":"p"}]`
	err := c.PublishTally(ctx, "e1", results, 2)
	if err == nil {
		t.Fatal("PublishTally should reject unknown candidate")
	}
}

func TestPublishTallyRejectsEmptyProof(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.FreezeElection(ctx, "e1"))

	results := `[{"candidate":"A","sum":1,"proof":""},{"candidate":"B","sum":1,"proof":"p"}]`
	err := c.PublishTally(ctx, "e1", results, 2)
	if err == nil {
		t.Fatal("PublishTally should reject missing proof")
	}
}

func TestGetTallyNotPublished(t *testing.T) {
	ctx, _ := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))

	_, err := c.GetTally(ctx, "e1")
	if err == nil {
		t.Fatal("GetTally should fail before publish")
	}
}

func TestPublishTallyEmitsEvent(t *testing.T) {
	ctx, stub := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.FreezeElection(ctx, "e1"))

	results := `[{"candidate":"A","sum":3,"proof":"p"},{"candidate":"B","sum":2,"proof":"p"}]`
	must(t, c.PublishTally(ctx, "e1", results, 5))

	events := stub.Events()
	found := false
	for _, e := range events {
		if e.EventName == "TallyPublished" {
			found = true
			if !strings.Contains(string(e.Payload), "e1") {
				t.Errorf("event payload missing electionID: %s", e.Payload)
			}
		}
	}
	if !found {
		t.Error("TallyPublished event was not emitted")
	}
}

func TestBallotCastEventEmitted(t *testing.T) {
	ctx, stub := testContext(t)
	c := &BallotContract{}

	must(t, c.RegisterElection(ctx, "e1", "E", `["A","B"]`, "pk"))
	must(t, c.StartVoting(ctx, "e1"))
	must(t, c.SubmitBallot(ctx, "e1", "b1", makeEncryptedVotes("1,2", "3,4"), "t1"))

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
