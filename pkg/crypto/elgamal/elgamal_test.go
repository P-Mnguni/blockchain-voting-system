package elgamal

import (
	"math/big"
	"testing"
)

func TestElGamalEncryptDecryptVote(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	ct0, err := sk.EncryptVote(0)
	if err != nil {
		t.Fatalf("EncryptVote(0) failed: %v", err)
	}

	v0, err := sk.DecryptVote(ct0, 100)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}
	if v0 != 0 {
		t.Errorf("Expected 0, got %d", v0)
	}

	ct1, err := sk.EncryptVote(1)
	if err != nil {
		t.Fatalf("EncryptVote(1) failed: %v", err)
	}

	v1, err := sk.DecryptVote(ct1, 100)
	if err != nil {
		t.Fatalf("DecryptVote failed for 1: %v", err)
	}

	if v1 != 1 {
		t.Errorf("Expected 1, got %d", v1)
	}
}

func TestHomomorphicAddition(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	messages := []int64{1, 0, 1, 0, 1}
	ciphertexts := make([]*Ciphertext, len(messages))

	for i, m := range messages {
		ct, err := sk.EncryptVote(m)
		if err != nil {
			t.Fatalf("EncryptVote failed: %v", err)
		}
		ciphertexts[i] = ct
	}

	result, err := AddMany(ciphertexts, sk.P)
	if err != nil {
		t.Fatalf("AddMany failed: %v", err)
	}

	decrypted, err := sk.DecryptVote(result, 100)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}

	expected := int64(3)
	if decrypted != expected {
		t.Errorf("Homomorphic addition failed. Got %d, want %d", decrypted, expected)
	}
}

func TestHomomorphicAdditionLargeVote(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	const totalVotes = 1000
	ciphertexts := make([]*Ciphertext, totalVotes)
	expectedSum := int64(0)

	for i := 0; i < totalVotes; i++ {
		var vote int64
		if i < 700 {
			vote = 1
		} else {
			vote = 0
		}
		expectedSum += vote

		ct, err := sk.EncryptVote(vote)
		if err != nil {
			t.Fatalf("EncryptVote failed: %v", err)
		}
		ciphertexts[i] = ct
	}

	result, err := AddMany(ciphertexts, sk.P)
	if err != nil {
		t.Fatalf("AddMany failed: %v", err)
	}

	decrypted, err := sk.DecryptVote(result, 2000)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}

	if decrypted != expectedSum {
		t.Errorf("Homomorphic addition failed. Got %d, want %d", decrypted, expectedSum)
	}
}

func TestScalarMultiplication(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	ct, err := sk.EncryptVote(1)
	if err != nil {
		t.Fatalf("EncryptVote failed: %v", err)
	}

	scaled := ScalarMul(ct, 5, sk.P)
	decrypted, err := sk.DecryptVote(scaled, 100)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}

	if decrypted != 5 {
		t.Errorf("Scalar multiplication failed, Got %d, want 5", decrypted)
	}
}

func TestMultipleEncryptSameVote(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	c1, _ := sk.EncryptVote(1)
	c2, _ := sk.EncryptVote(1)

	if c1.C1.Cmp(c2.C1) == 0 || c1.C2.Cmp(c2.C2) == 0 {
		t.Error("Encrypting same vote should produce different ciphertexts (randomized)")
	}

	v1, _ := sk.DecryptVote(c1, 10)
	v2, _ := sk.DecryptVote(c2, 10)
	if v1 != v2 {
		t.Errorf("Decrypted votes differ: %d vs %d", v1, v2)
	}
}

func TestEncryptRejectsZero(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	_, err = sk.Encrypt(big.NewInt(0))
	if err == nil {
		t.Error("Encrypt(0) should fail - use EncryptVote instead")
	}
}

func BenchmarkEncryptVote(b *testing.B) {
	sk, _ := NewElGamal(2048)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.EncryptVote(1)
	}
}

func BenchmarkDecryptVote(b *testing.B) {
	sk, _ := NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.DecryptVote(ct, 100)
	}
}

func BenchmarkAddMany(b *testing.B) {
	sk, _ := NewElGamal(2048)
	ciphertexts := make([]*Ciphertext, 100)
	for i := 0; i < 100; i++ {
		ciphertexts[i], _ = sk.EncryptVote(1)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AddMany(ciphertexts, sk.P)
	}
}
