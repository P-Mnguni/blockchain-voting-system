package zkproofs

import (
	"math/big"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
)

func TestDecryptionProof(t *testing.T) {
	sk, err := elgamal.NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate ElGamal keys: %v", err)
	}

	ct, err := sk.EncryptVote(1)
	if err != nil {
		t.Fatalf("EncryptVote failed: %v", err)
	}

	m := new(big.Int).Exp(sk.G, big.NewInt(1), sk.P)

	proof, err := GenerateDecryptionProof(
		ct.C1, ct.C2, m,
		sk.Y, sk.P, sk.G,
		sk.X,
	)
	if err != nil {
		t.Fatalf("GenerateDecryptionProof failed: %v", err)
	}

	if !VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Error("Decryption proof verification failed")
	}

	wrongM := new(big.Int).Exp(sk.G, big.NewInt(0), sk.P)
	if VerifyDecryptionProof(ct.C1, ct.C2, wrongM, sk.Y, sk.P, sk.G, proof) {
		t.Error("Proof should fail for wrong plaintext")
	}
}

func TestTallyProof(t *testing.T) {
	sk, err := elgamal.NewElGamal(2048)
	if err != nil {
		t.Fatalf("Failed to generate ElGamal keys: %v", err)
	}

	ctA1, _ := sk.EncryptVote(1)
	ctA2, _ := sk.EncryptVote(1)
	sumA := elgamal.Add(ctA1, ctA2, sk.P)

	ctB1, _ := sk.EncryptVote(1)
	ctB0, _ := sk.EncryptVote(0)
	sumB := elgamal.Add(ctB1, ctB0, sk.P)

	perCandidate := []struct{ C1, C2 *big.Int }{
		{C1: sumA.C1, C2: sumA.C2},
		{C1: sumB.C1, C2: sumB.C2},
	}
	results := []*big.Int{big.NewInt(2), big.NewInt(1)}

	pubKey := &struct{ P, G, Y *big.Int }{P: sk.P, G: sk.G, Y: sk.Y}

	proof, err := GenerateTallyProof(perCandidate, results, pubKey, sk.X)
	if err != nil {
		t.Fatalf("GenerateTallyProof failed: %v", err)
	}

	if !VerifyTallyProof(perCandidate, results, pubKey, proof) {
		t.Error("Tally proof verification failed")
	}

	wrongResults := []*big.Int{big.NewInt(3), big.NewInt(1)}
	if VerifyTallyProof(perCandidate, wrongResults, pubKey, proof) {
		t.Error("Tally proof should fail with tampered results")
	}
}

func TestDecryptionProofWrongKey(t *testing.T) {
	sk1, _ := elgamal.NewElGamal(2048)
	sk2, _ := elgamal.NewElGamal(2048)

	ct, _ := sk1.EncryptVote(1)
	m := new(big.Int).Exp(sk1.G, big.NewInt(1), sk1.P)

	proof, err := GenerateDecryptionProof(
		ct.C1, ct.C2, m,
		sk1.Y, sk1.P, sk1.G,
		sk1.X,
	)
	if err != nil {
		t.Fatalf("GenerateDecryptionProof failed: %v", err)
	}

	if VerifyDecryptionProof(ct.C1, ct.C2, m, sk2.Y, sk1.P, sk1.G, proof) {
		t.Error("Proof should fail when verified against a different public key")
	}
}

func BenchmarkGenerateDecryptionProof(b *testing.B) {
	sk, _ := elgamal.NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)
	m := new(big.Int).Exp(sk.G, big.NewInt(1), sk.P)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, sk.X)
	}
}

func BenchmarkVerifyDecryptionProof(b *testing.B) {
	sk, _ := elgamal.NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)
	m := new(big.Int).Exp(sk.G, big.NewInt(1), sk.P)
	proof, _ := GenerateDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, sk.X)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof)
	}
}
