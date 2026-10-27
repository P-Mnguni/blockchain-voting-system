package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestMockEIDAccepts(t *testing.T) {
	m := NewMockEID()
	if err := m.Verify("voter-001"); err != nil {
		t.Errorf("voter-001 should be accepted, got %v", err)
	}
}

func TestMockEIDRejects(t *testing.T) {
	m := NewMockEID()
	err := m.Verify("voter-999")
	if err == nil {
		t.Fatal("voter-999 should be rejected")
	}
	var eidErr *EIDError
	if !errors.As(err, &eidErr) {
		t.Errorf("expected *EIDError, got %T", err)
	}
}

func TestEIDErrorMessageMasksID(t *testing.T) {
	m := NewMockEID()
	err := m.Verify("voter-secret-999")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()

	// 1. Must not leak the full voter ID
	if strings.Contains(msg, "voter-secret-999") {
		t.Errorf("error message leaked the full voter ID: %s", msg)
	}

	// 2. Must contain masking asterisks
	if !strings.Contains(msg, "*") {
		t.Errorf("error message should contain masking asterisks: %s", msg)
	}

	// 3. Mask should preserve exactly 2 leading and 2 trailing chars
	//    (per maskVoterID's contract)
	if !strings.Contains(msg, "vo") {
		t.Errorf("error message should preserve first 2 chars: %s", msg)
	}
	if !strings.Contains(msg, "99") {
		t.Errorf("error message should preserve last 2 chars: %s", msg)
	}

	// 4. Length preserved (masking replaces, doesn't truncate)
	if !strings.Contains(msg, "vo************99") {
		t.Errorf("error message should be vo************99: %s", msg)
	}
}
