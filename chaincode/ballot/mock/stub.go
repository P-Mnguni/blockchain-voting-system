// Package mock provides a minimal in-memory implementation of the Fabric
// ChaincodeStubInterface for unit testing BallotContract without a live peer.
package mock

import (
	"sort"
	"sync"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/peer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: Stub must satisfy the Fabric interface.
var _ shim.ChaincodeStubInterface = (*Stub)(nil)

// Stub is a minimal in-memory stub. Only methods used by BallotContract
// behave meaningfully; the rest return zero values or panic.
type Stub struct {
	mu        sync.Mutex
	state     map[string][]byte
	events    []*peer.ChaincodeEvent
	txID      string
	channel   string
	timestamp *timestamppb.Timestamp
}

// NewStub returns a fresh mock stub.
func NewStub() *Stub {
	return &Stub{
		state:     make(map[string][]byte),
		txID:      "mock-tx-1",
		channel:   "mock-channel",
		timestamp: timestamppb.New(time.Unix(1700000000, 0)),
	}
}

// --- state methods (used by BallotContract) ---

func (s *Stub) GetState(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.state[key]
	if !ok {
		return nil, nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (s *Stub) PutState(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := make([]byte, len(value))
	copy(stored, value)
	s.state[key] = stored
	return nil
}

func (s *Stub) DelState(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state, key)
	return nil
}

func (s *Stub) GetStateByRange(startKey, endKey string) (shim.StateQueryIteratorInterface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keys []string
	for k := range s.state {
		if k >= startKey && k < endKey {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	// Snapshot the values so mutation during iteration doesn't affect results.
	snapshot := make(map[string][]byte, len(keys))
	for _, k := range keys {
		v := s.state[k]
		cp := make([]byte, len(v))
		copy(cp, v)
		snapshot[k] = cp
	}
	return &rangeIterator{keys: keys, state: snapshot}, nil
}

// --- event + tx metadata (used by BallotContract) ---

func (s *Stub) SetEvent(name string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	s.events = append(s.events, &peer.ChaincodeEvent{
		EventName: name,
		Payload:   cp,
	})
	return nil
}

func (s *Stub) GetTxTimestamp() (*timestamppb.Timestamp, error) {
	return s.timestamp, nil
}

func (s *Stub) GetChannelID() string { return s.channel }
func (s *Stub) GetTxID() string      { return s.txID }

// --- test helpers (not part of the interface) ---

// Events returns the events emitted so far.
func (s *Stub) Events() []*peer.ChaincodeEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events
}

// Keys returns all keys in the stub, sorted. For debugging.
func (s *Stub) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.state))
	for k := range s.state {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- range iterator ---

type rangeIterator struct {
	keys  []string
	state map[string][]byte
	idx   int
}

func (it *rangeIterator) HasNext() bool { return it.idx < len(it.keys) }
func (it *rangeIterator) Close() error  { return nil }

func (it *rangeIterator) Next() (*queryresult.KV, error) {
	if !it.HasNext() {
		return nil, nil
	}
	k := it.keys[it.idx]
	it.idx++
	return &queryresult.KV{Key: k, Value: it.state[k]}, nil
}

// --- methods not used by BallotContract ---
// We return zero values rather than panic so tests that don't exercise
// these paths still run. If you want to be stricter, replace with panics.

func (s *Stub) GetArgs() [][]byte                            { return nil }
func (s *Stub) GetStringArgs() []string                      { return nil }
func (s *Stub) GetFunctionAndParameters() (string, []string) { return "", nil }
func (s *Stub) GetArgsSlice() ([]byte, error)                { return nil, nil }
func (s *Stub) InvokeChaincode(string, [][]byte, string) peer.Response {
	return peer.Response{}
}

func (s *Stub) SetStateValidationParameter(string, []byte) error   { return nil }
func (s *Stub) GetStateValidationParameter(string) ([]byte, error) { return nil, nil }

func (s *Stub) GetStateByRangeWithPagination(string, string, int32, string) (shim.StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	return nil, nil, nil
}
func (s *Stub) GetStateByPartialCompositeKey(string, []string) (shim.StateQueryIteratorInterface, error) {
	return nil, nil
}
func (s *Stub) GetStateByPartialCompositeKeyWithPagination(string, []string, int32, string) (shim.StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	return nil, nil, nil
}
func (s *Stub) CreateCompositeKey(string, []string) (string, error) { return "", nil }
func (s *Stub) SplitCompositeKey(string) (string, []string, error)  { return "", nil, nil }

func (s *Stub) GetQueryResult(string) (shim.StateQueryIteratorInterface, error) {
	return nil, nil
}
func (s *Stub) GetQueryResultWithPagination(string, int32, string) (shim.StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	return nil, nil, nil
}
func (s *Stub) GetHistoryForKey(string) (shim.HistoryQueryIteratorInterface, error) {
	return nil, nil
}

func (s *Stub) GetPrivateData(string, string) ([]byte, error)                  { return nil, nil }
func (s *Stub) GetPrivateDataHash(string, string) ([]byte, error)              { return nil, nil }
func (s *Stub) PutPrivateData(string, string, []byte) error                    { return nil }
func (s *Stub) DelPrivateData(string, string) error                            { return nil }
func (s *Stub) PurgePrivateData(string, string) error                          { return nil }
func (s *Stub) SetPrivateDataValidationParameter(string, string, []byte) error { return nil }
func (s *Stub) GetPrivateDataValidationParameter(string, string) ([]byte, error) {
	return nil, nil
}
func (s *Stub) GetPrivateDataByRange(string, string, string) (shim.StateQueryIteratorInterface, error) {
	return nil, nil
}
func (s *Stub) GetPrivateDataByPartialCompositeKey(string, string, []string) (shim.StateQueryIteratorInterface, error) {
	return nil, nil
}
func (s *Stub) GetPrivateDataQueryResult(string, string) (shim.StateQueryIteratorInterface, error) {
	return nil, nil
}

func (s *Stub) GetCreator() ([]byte, error)                      { return nil, nil }
func (s *Stub) GetTransient() (map[string][]byte, error)         { return nil, nil }
func (s *Stub) GetBinding() ([]byte, error)                      { return nil, nil }
func (s *Stub) GetDecorations() map[string][]byte                { return nil }
func (s *Stub) GetSignedProposal() (*peer.SignedProposal, error) { return nil, nil }
