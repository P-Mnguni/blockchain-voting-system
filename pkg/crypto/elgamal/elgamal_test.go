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

func TestDecryptVoteDiagnostic(t *testing.T) {
	sk, err := NewElGamal(2048)
	if err != nil {
		t.Fatalf("NewElGamal failed: %v", err)
	}

	t.Logf("g = %s", sk.G.Text(16))
	t.Logf("p bit length = %d", sk.P.BitLen())

	for _, vote := range []int64{0, 1, 2, 3, 5, 10} {
		ct, _ := sk.EncryptVote(vote)
		encoded, _ := sk.Decrypt(ct)
		expected := new(big.Int).Exp(sk.G, big.NewInt(vote), sk.P)
		t.Logf("vote=%d: encoded=%s expected=%s match=%v",
			vote,
			truncateHex(encoded, 8),
			truncateHex(expected, 8),
			encoded.Cmp(expected) == 0,
		)

		got, err := sk.DecryptVote(ct, 100)
		if err != nil {
			t.Errorf("vote=%d: DecryptVote err=%v", vote, err)
		} else if got != vote {
			t.Errorf("vote=%d: DecryptVote got %d", vote, got)
		}
	}

	ciphertexts := make([]*Ciphertext, 100)
	for i := range ciphertexts {
		ciphertexts[i], _ = sk.EncryptVote(1)
	}
	sum, err := AddMany(ciphertexts, sk.P)
	if err != nil {
		t.Fatalf("AddMany failed: %v", err)
	}

	encoded, _ := sk.Decrypt(sum)
	t.Logf("sum-of-100: encoded = %s", truncateHex(encoded, 8))
	t.Logf("sum-of-100: g^99	= %s", truncateHex(new(big.Int).Exp(sk.G, big.NewInt(99), sk.P), 8))
	t.Logf("sum-of-100: g^100	= %s", truncateHex(new(big.Int).Exp(sk.G, big.NewInt(100), sk.P), 8))
	t.Logf("sum-of-100: g^101	= %s", truncateHex(new(big.Int).Exp(sk.G, big.NewInt(101), sk.P), 8))

	for _, bound := range []int64{99, 100, 101} {
		got, err := sk.DecryptVote(sum, bound)
		t.Logf("DecryptVot(sum, %d) = %d, err = %v", bound, got, err)
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
