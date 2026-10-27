package fabricclient

import (
	"errors"
	"testing"
)

func TestMockClientSubmit(t *testing.T) {
	m := &MockClient{
		SubmitFn: func(fn string, args ...string) ([]byte, error) {
			return []byte("ok:" + fn), nil
		},
	}
	out, err := m.Submit("SubmitBallot", "e1", "b1")
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if string(out) != "ok:SubmitBallot" {
		t.Errorf("unexpected output: %s", out)
	}
	if len(m.SubmitCalls) != 1 || m.SubmitCalls[0].Fn != "SubmitBallot" {
		t.Errorf("call not recorded")
	}
}

func TestMockClientEvaluate(t *testing.T) {
	m := &MockClient{
		EvalFn: func(fn string, args ...string) ([]byte, error) {
			return []byte("false"), nil
		},
	}
	out, err := m.Evaluate("HasTokenBeenSpent", "e1", "hash")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if string(out) != "false" {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestMockClientUnsetFunctions(t *testing.T) {
	m := &MockClient{}
	if _, err := m.Submit("x"); err == nil {
		t.Error("Submit with nil SubmitFn should error")
	}
	if _, err := m.Evaluate("x"); err == nil {
		t.Error("Evaluate with nil EvalFn should error")
	}
}

func TestMockClientPropagatesErrors(t *testing.T) {
	sentinel := errors.New("boom")
	m := &MockClient{
		SubmitFn: func(string, ...string) ([]byte, error) { return nil, sentinel },
	}
	_, err := m.Submit("x")
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}
