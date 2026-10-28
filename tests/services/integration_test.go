package services

import (
	"encoding/json"
	"testing"
)

type registerReq struct {
	VoterID    string `json:"voterId"`
	ElectionID string `json:"electionId"`
}

type registerResp struct {
	Credential string `json:"credential"`
	Expiry     int64  `json:"expiry"`
}

type castReq struct {
	ElectionID      string `json:"electionId"`
	EncryptedBallot string `json:"encryptedBallot"`
	Credential      string `json:"credential"`
}

type castResp struct {
	BallotID    string `json:"ballotId"`
	TxID        string `json:"txId"`
	CommittedAt int64  `json:"committedAt"`
}

func TestHealthz(t *testing.T) {
	idURL := Env(t, "IDENTITY_URL")
	castURL := Env(t, "CASTING_URL")

	id := NewHTTPClient(idURL)
	if code, err := id.Get("/healthz"); err != nil || code != 200 {
		t.Fatalf("identity /healthz: code=%d err=%v", code, err)
	}

	cast := NewHTTPClient(castURL)
	if code, err := cast.Get("/healthz"); err != nil || code != 200 {
		t.Fatalf("casting /healthz: code=%d err=%v", code, err)
	}
}

func TestRegisterAndCast(t *testing.T) {
	idURL := Env(t, "IDENTITY_URL")
	castURL := Env(t, "CASTING_URL")
	electionID := Env(t, "ELECTION_ID")
	voterID := Env(t, "TEST_VOTER_ID")

	id := NewHTTPClient(idURL)
	cast := NewHTTPClient(castURL)

	// 1. Register
	var reg registerResp
	code, err := id.PostJSON("/register", registerReq{
		VoterID:    voterID,
		ElectionID: electionID,
	}, &reg)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if code != 200 {
		t.Fatalf("register: HTTP %d", code)
	}
	if reg.Credential == "" {
		t.Fatal("register returned empty credential")
	}

	// 2. Cast a ballot
	encryptedBallot, _ := json.Marshal(map[string]string{"C1": "10", "C2": "20"})
	var receipt castResp
	code, err = cast.PostJSON("/cast", castReq{
		ElectionID:      electionID,
		EncryptedBallot: string(encryptedBallot),
		Credential:      reg.Credential,
	}, &receipt)
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	if code != 200 {
		t.Fatalf("cast: HTTP %d", code)
	}
	if receipt.BallotID == "" {
		t.Fatal("cast returned empty ballotID")
	}
	if receipt.CommittedAt == 0 {
		t.Error("cast returned zero committedAt")
	}
	t.Logf("receipt: ballotID=%s committedAt=%d", receipt.BallotID, receipt.CommittedAt)
}

func TestCastRejectsDuplicateCredential(t *testing.T) {
	idURL := Env(t, "IDENTITY_URL")
	castURL := Env(t, "CASTING_URL")
	electionID := Env(t, "ELECTION_ID")
	voterID := Env(t, "TEST_VOTER_ID")

	id := NewHTTPClient(idURL)
	cast := NewHTTPClient(castURL)

	var reg registerResp
	if _, err := id.PostJSON("/register", registerReq{VoterID: voterID, ElectionID: electionID}, &reg); err != nil {
		t.Fatalf("register: %v", err)
	}

	encryptedBallot, _ := json.Marshal(map[string]string{"C1": "10", "C2": "20"})
	body := castReq{
		ElectionID:      electionID,
		EncryptedBallot: string(encryptedBallot),
		Credential:      reg.Credential,
	}

	var r1 castResp
	if _, err := cast.PostJSON("/cast", body, &r1); err != nil {
		t.Fatalf("first cast: %v", err)
	}

	code, err := cast.PostJSON("/cast", body, nil)
	if err == nil || code < 400 {
		t.Fatalf("second cast should fail, got code=%d err=%v", code, err)
	}
	t.Logf("correctly rejected: code=%d", code)
}

func TestCastRejectsGarbageCredential(t *testing.T) {
	castURL := Env(t, "CASTING_URL")
	electionID := Env(t, "ELECTION_ID")

	cast := NewHTTPClient(castURL)
	encryptedBallot, _ := json.Marshal(map[string]string{"C1": "10", "C2": "20"})

	code, err := cast.PostJSON("/cast", castReq{
		ElectionID:      electionID,
		EncryptedBallot: string(encryptedBallot),
		Credential:      "garbage",
	}, nil)
	if err == nil || code < 400 {
		t.Fatalf("garbage credential should be rejected, got code=%d err=%v", code, err)
	}
}

func TestRegisterRejectsUnknownVoter(t *testing.T) {
	idURL := Env(t, "IDENTITY_URL")
	electionID := Env(t, "ELECTION_ID")

	id := NewHTTPClient(idURL)
	code, err := id.PostJSON("/register", registerReq{
		VoterID:    "not-a-real-voter",
		ElectionID: electionID,
	}, nil)
	if err == nil || code != 401 {
		t.Fatalf("unknown voter should return 401, got code=%d err=%v", code, err)
	}
}
