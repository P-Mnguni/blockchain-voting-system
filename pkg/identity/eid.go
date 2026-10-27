package identity

import "strings"

// EIDProvider verifies a voter's national electronic ID.
// In production this calls the national eID service; for Milestone 3 we use
// MockEID.
type EIDProvider interface {
	Verify(voterID string) error
}

// MockEID accepts a fixed list of test voter IDs. Any other ID is rejected.
type MockEID struct {
	Valid map[string]bool
}

// NewMockEID returns a mock provider pre-seeded with demo voter IDs.
func NewMockEID() *MockEID {
	return &MockEID{
		Valid: map[string]bool{
			"voter-001": true,
			"voter-002": true,
			"voter-003": true,
		},
	}
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
