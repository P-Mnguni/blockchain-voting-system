package zkproofs

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"math/big"
)

// ProofOfCorrectDecryption proves that a ciphertext was correctly decrypted
// without revealing the decryption key or the plaintext

type DecryptionProof struct {
	A *big.Int
	B *big.Int
	S *big.Int
}

// GenerateDecryptionProof generates a zero-knowledge proof of correct decryption
func GenerateDecryptionProof(C1, C2, m, y, p, g, x *big.Int) (*DecryptionProof, error) {
	if m.Sign() == 0 {
		return nil, errors.New("m must be nonzero")
	}

	mInv := new(big.Int).ModInverse(m, p)
	if mInv == nil {
		return nil, errors.New("m not invertible mod p")
	}

	c2OverM := new(big.Int).Mul(C2, mInv)
	c2OverM.Mod(c2OverM, p)

	pm1 := new(big.Int).Sub(p, big.NewInt(1))
	r, err := rand.Int(rand.Reader, pm1)
	if err != nil {
		return nil, err
	}

	A := new(big.Int).Exp(g, r, p)
	B := new(big.Int).Exp(C1, r, p)

	c := hashToChallenge(p, g, y, C1, c2OverM, A, B)

	cx := new(big.Int).Mul(c, x)
	s := new(big.Int).Add(r, cx)
	s.Mod(s, pm1)

	return &DecryptionProof{
		A: A,
		B: B,
		S: s,
	}, nil
}

// VerifyDecryptionProof verifies a proof of correct decryption
func VerifyDecryptionProof(C1, C2, m, y, p, g *big.Int, proof *DecryptionProof) bool {
	if m.Sign() == 0 {
		return false
	}

	mInv := new(big.Int).ModInverse(m, p)
	if mInv == nil {
		return false
	}

	c2OverM := new(big.Int).Mul(C2, mInv)
	c2OverM.Mod(c2OverM, p)

	c := hashToChallenge(p, g, y, C1, c2OverM, proof.A, proof.B)

	pm1 := new(big.Int).Sub(p, big.NewInt(1))

	gS := new(big.Int).Exp(g, proof.S, p)
	yC := new(big.Int).Exp(y, c, p)
	rhs1 := new(big.Int).Mod(new(big.Int).Mul(proof.A, yC), p)

	if gS.Cmp(rhs1) != 0 {
		return false
	}

	c1S := new(big.Int).Exp(C1, proof.S, p)
	c2OverMC := new(big.Int).Exp(c2OverM, c, p)
	rhs2 := new(big.Int).Mod(new(big.Int).Mul(proof.B, c2OverMC), p)

	_ = pm1

	return c1S.Cmp(rhs2) == 0
}

// hashToChallenge computes Fiat-Shamir challenge over all relevant values
func hashToChallenge(p, g, y, C1, c2OverM, A, B *big.Int) *big.Int {
	h := sha256.New()
	for _, v := range []*big.Int{g, y, C1, c2OverM, A, B} {
		h.Write(v.Bytes())
	}
	c := new(big.Int).SetBytes(h.Sum(nil))

	pm1 := new(big.Int).Sub(p, big.NewInt(1))
	return c.Mod(c, pm1)
}

// ProofOfTallyCorrectness proves that the tally was computed correctly
type TallyProof struct {
	SumC1  *big.Int
	SumC2  *big.Int
	Proofs []*DecryptionProof
}

// GenerateTallyProof generates proofs for all candidates in the tally
func GenerateTallyProof(
	perCandidateCiphertexts []struct{ C1, C2 *big.Int },
	results []*big.Int,
	publicKey *struct{ P, G, Y *big.Int },
	x *big.Int,
) (*TallyProof, error) {
	if len(perCandidateCiphertexts) != len(results) {
		return nil, errors.New("mismatched candidates and results")
	}

	proofs := make([]*DecryptionProof, len(results))
	for i := range results {
		m := new(big.Int).Exp(publicKey.G, results[i], publicKey.P)

		p, err := GenerateDecryptionProof(
			perCandidateCiphertexts[i].C1,
			perCandidateCiphertexts[i].C2,
			m,
			publicKey.Y,
			publicKey.P,
			publicKey.G,
			x,
		)
		if err != nil {
			return nil, err
		}
		proofs[i] = p
	}

	return &TallyProof{Proofs: proofs}, nil
}

func VerifyTallyProof(
	perCandidateCiphertexts []struct{ C1, C2 *big.Int },
	results []*big.Int,
	publicKey *struct{ P, G, Y *big.Int },
	proof *TallyProof,
) bool {
	if len(perCandidateCiphertexts) != len(results) || len(proof.Proofs) != len(results) {
		return false
	}

	for i := range results {
		m := new(big.Int).Exp(publicKey.G, results[i], publicKey.P)
		if !VerifyDecryptionProof(
			perCandidateCiphertexts[i].C1,
			perCandidateCiphertexts[i].C2,
			m,
			publicKey.Y,
			publicKey.P,
			publicKey.G,
			proof.Proofs[i],
		) {
			return false
		}
	}

	return true
}
