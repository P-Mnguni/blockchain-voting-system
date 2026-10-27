package identity

import (
	"errors"
	"sync"
	"testing"
)

func TestMemoryVoterStoreMarkAndCheck(t *testing.T) {
	s := NewMemoryVoterStore()
	if s.IsRegistered("v1", "e1") {
		t.Error("v1 should not be registered yet")
	}
	if err := s.MarkRegistered("v1", "e1"); err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}
	if !s.IsRegistered("v1", "e1") {
		t.Error("v1 should now be registered")
	}
}

func TestMemoryVoterStoreDuplicate(t *testing.T) {
	s := NewMemoryVoterStore()
	_ = s.MarkRegistered("v1", "e1")
	err := s.MarkRegistered("v1", "e1")
	if err == nil {
		t.Fatal("duplicate registration should fail")
	}
	var are *AlreadyRegisteredError
	if !errors.As(err, &are) {
		t.Errorf("expected *AlreadyRegisteredError, got %T", err)
	}
}

func TestMemoryVoterStoreDifferentElections(t *testing.T) {
	s := NewMemoryVoterStore()
	_ = s.MarkRegistered("v1", "e1")
	if err := s.MarkRegistered("v1", "e2"); err != nil {
		t.Errorf("same voter in different election should be allowed: %v", err)
	}
}

func TestMemoryVoterStoreConcurrent(t *testing.T) {
	s := NewMemoryVoterStore()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.MarkRegistered("v1", "e1")
		}()
	}
	wg.Wait()
	// Exactly one should have succeeded; we can't easily check which, but at
	// least the store should not have crashed and the entry exists.
	if !s.IsRegistered("v1", "e1") {
		t.Error("v1 should be registered")
	}
}
