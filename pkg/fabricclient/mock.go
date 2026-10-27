package fabricclient

import (
	"fmt"
	"sync"
)

// MockClient is an in-memory Client for unit tests. Submit and Evaluate are
// backed by user-provided functions, so each test can define its own ledger
// behavior.
type MockClient struct {
	mu       sync.Mutex
	SubmitFn func(fn string, args ...string) ([]byte, error)
	EvalFn   func(fn string, args ...string) ([]byte, error)

	// Captured calls, useful for assertions.
	SubmitCalls []MockCall
	EvalCalls   []MockCall
}

type MockCall struct {
	Fn   string
	Args []string
}

func (m *MockClient) Submit(fn string, args ...string) ([]byte, error) {
	m.mu.Lock()
	m.SubmitCalls = append(m.SubmitCalls, MockCall{Fn: fn, Args: append([]string(nil), args...)})
	m.mu.Unlock()
	if m.SubmitFn == nil {
		return nil, fmt.Errorf("mock: SubmitFn not set")
	}
	return m.SubmitFn(fn, args...)
}

func (m *MockClient) Evaluate(fn string, args ...string) ([]byte, error) {
	m.mu.Lock()
	m.EvalCalls = append(m.EvalCalls, MockCall{Fn: fn, Args: append([]string(nil), args...)})
	m.mu.Unlock()
	if m.EvalFn == nil {
		return nil, fmt.Errorf("mock: EvalFn not set")
	}
	return m.EvalFn(fn, args...)
}

func (m *MockClient) Close() error { return nil }
