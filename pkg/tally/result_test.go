package tally

import (
	"math/big"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
)

func TestSerializeDeserializeCandidateResults(t *testing.T) {
	original := []CandidateResult{
		{
			Candidate: "Alice",
			Sum:       10,
			Proof: &zkproofs.DecryptionProof{
				A: big.NewInt(111),
				B: big.NewInt(222),
				S: big.NewInt(333),
			},
		},
		{
			Candidate: "Bob",
			Sum:       5,
			Proof: &zkproofs.DecryptionProof{
				A: big.NewInt(444),
				B: big.NewInt(555),
				S: big.NewInt(666),
			},
		},
	}

	serialized, err := serializeCandidateResults(original)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	deserialized, err := deserializeCandidateResults(serialized)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if len(deserialized) != len(original) {
		t.Fatalf("count mismatch: %d vs %d", len(deserialized), len(original))
	}
	for i := range original {
		if deserialized[i].Candidate != original[i].Candidate {
			t.Errorf("candidate mismatch at %d", i)
		}
		if deserialized[i].Sum != original[i].Sum {
			t.Errorf("sum mismatch at %d", i)
		}
		if deserialized[i].Proof.A.Cmp(original[i].Proof.A) != 0 {
			t.Errorf("proof.A mismatch at %d", i)
		}
		if deserialized[i].Proof.S.Cmp(original[i].Proof.S) != 0 {
			t.Errorf("proof.S mismatch at %d", i)
		}
	}
}

func TestSerializeRejectsNilProof(t *testing.T) {
	results := []CandidateResult{
		{Candidate: "Alice", Sum: 10, Proof: nil},
	}
	_, err := serializeCandidateResults(results)
	if err == nil {
		t.Fatal("expected error for nil proof")
	}
}
