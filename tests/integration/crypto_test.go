package integration

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto"
)

func TestCryptoSurface(t *testing.T) {
	t.Run("Shamir", func(t *testing.T) {
		secret := []byte("re-export-test-secret-0123456789")
		shares, err := crypto.SplitShamir(secret, 5, 3)
		if err != nil {
			t.Fatalf("SplitShamir failed: %v", err)
		}
		reconstructed, err := crypto.CombineShamir(shares[:3])
		if err != nil {
			t.Fatalf("CombineShamir failed: %v", err)
		}
		if !bytes.Equal(reconstructed, secret) {
			t.Error("Shamir round-trip failed via crypto package exports")
		}
	})

	t.Run("ElGamal", func(t *testing.T) {
		sk, err := crypto.NewElGamal(2048)
		if err != nil {
			t.Fatalf("NewElGamal failed: %v", err)
		}

		ct1, err := sk.EncryptVote(1)
		if err != nil {
			t.Fatalf("EncryptVote failed: %v", err)
		}
		ct0, err := sk.EncryptVote(0)
		if err != nil {
			t.Fatalf("EncryptVote(0) failed: %v", err)
		}

		// Homomorphic add
		ct2, _ := sk.EncryptVote(1)
		sum := crypto.ElGamalAddMany([]*crypto.Ciphertext{ct1, ct2, ct0}, sk.P)

		result, err := sk.DecryptVote(sum, 100)
		if err != nil {
			t.Fatalf("DecryptVote failed: %v", err)
		}
		if result != 2 {
			t.Errorf("Expected sum 2, got %d", result)
		}
	})

	t.Run("BlindSignatures", func(t *testing.T) {
		issuer, err := crypto.NewBlindIssuer(2048)
		if err != nil {
			t.Fatalf("NewBlindIssuer failed: %v", err)
		}
		voterID := []byte("voter-42")
		cred, err := issuer.IssueCredential(voterID)
		if err != nil {
			t.Fatalf("IssueCredential failed: %v", err)
		}
		if !issuer.VerifyCredential(voterID, cred) {
			t.Error("Credential verification failed via crypto package exports")
		}
	})

	t.Run("ZKProofs", func(t *testing.T) {
		sk, _ := crypto.NewElGamal(2048)
		ct, _ := sk.EncryptVote(1)
		m := new(big.Int).Exp(sk.G, big.NewInt(1), sk.P)

		proof, err := crypto.GenerateDecryptionProof(
			ct.C1, ct.C2, m,
			sk.Y, sk.P, sk.G, sk.X,
		)
		if err != nil {
			t.Fatalf("GenerateDecryptionProof failed: %v", err)
		}
		if !crypto.VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
			t.Error("ZK proof verification failed via crypto package exports")
		}
	})
}
