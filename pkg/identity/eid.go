package identity

import (
	"fmt"
	"strings"
)

// EIDProvider verifies a voter's national electronic ID.
type EIDProvider interface {
	Verify(voterID string) error
}

// MockEID accepts a fixed list of test voter IDs. Any other ID is rejected.
type MockEID struct {
	Valid map[string]bool
}

/*
NewMockEID returns a mock provider pre-seeded with 50 demo voter IDs
(voter-001 through voter-050).
*/
func NewMockEID() *MockEID {
	valid := make(map[string]bool, 50)
	for i := 1; i <= 50; i++ {
		valid[fmt.Sprintf("voter-%03d", i)] = true
	}
	return &MockEID{Valid: valid}
}

func (m *MockEID) Verify(voterID string) error {
	if !m.Valid[voterID] {
		return &EIDError{VoterID: voterID, Reason: "not in registry"}
	}
	return nil
}

// EIDError is returned when verification fails. Distinguishable for handler logic.
type EIDError struct {
	VoterID string
	Reason  string
}

func (e *EIDError) Error() string {
	return "eID verification failed for " + maskVoterID(e.VoterID) + ": " + e.Reason
}

// maskVoterID hides most of the ID in error messages.
func maskVoterID(id string) string {
	if len(id) <= 4 {
		return strings.Repeat("*", len(id))
	}
	return id[:2] + strings.Repeat("*", len(id)-4) + id[len(id)-2:]
}
