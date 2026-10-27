package identity

import (
	"sync"
)

// VoterStore tracks which voters have already registered for which elections.
// Prevents a voter from being issued a second credential for the same election.
type VoterStore interface {
	// MarkRegistered records that voterID has registered for electionID.
	// Returns an error if the voter is already registered for this election.
	MarkRegistered(voterID, electionID string) error
	// IsRegistered returns true if voterID has registered for electionID.
	IsRegistered(voterID, electionID string) bool
}

// MemoryVoterStore is an in-memory VoterStore.
type MemoryVoterStore struct {
	mu      sync.Mutex
	entries map[string]bool // key: electionID + "\x00" + voterID
}

func NewMemoryVoterStore() *MemoryVoterStore {
	return &MemoryVoterStore{entries: make(map[string]bool)}
}

func voterKey(electionID, voterID string) string {
	return electionID + "\x00" + voterID
}

func (m *MemoryVoterStore) MarkRegistered(voterID, electionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := voterKey(electionID, voterID)
	if m.entries[k] {
		return &AlreadyRegisteredError{VoterID: voterID, ElectionID: electionID}
	}
	m.entries[k] = true
	return nil
}

func (m *MemoryVoterStore) IsRegistered(voterID, electionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[voterKey(electionID, voterID)]
}

// AlreadyRegisteredError is returned when a voter registers twice for the same election.
type AlreadyRegisteredError struct {
	VoterID    string
	ElectionID string
}

func (e *AlreadyRegisteredError) Error() string {
	return "voter already registered for election " + e.ElectionID
}
