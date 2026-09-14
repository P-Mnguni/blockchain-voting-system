package zkproofs

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
)

// ProofOfCorrectDecryption proves that a ciphertext was correctly decrypted
// without revealing the decryption key or the plaintext

type DecryptionProof struct {
	Commitment *big.Int
	Challenge  *big.Int
	Response   *big.Int
}

// GenerateDecryptionProof generates a zero-knowledge proof of correct decryption
func GenerateDecryptionProof(C1, C2, m, y, p, g *big.Int, x *big.Int) (*DecryptionProof, error) {
	r, err := rand.Int(rand.Reader, p)
	if err != nil {
		return nil, err
	}

	t := new(big.Int).Exp(g, r, p)

	hash := sha256.Sum256(append(
		append(C1.Bytes(), C2.Bytes()...),
		append(m.Bytes(), t.Bytes()...)...,
	))
	challenge := new(big.Int).SetBytes(hash[:])
	challenge.Mod(challenge, p)

	challengeX := new(big.Int).Mul(challenge, x)
	response := new(big.Int).Sub(r, challengeX)
	response.Mod(response, new(big.Int).Sub(p, big.NewInt(1)))

	return &DecryptionProof{
		Commitment: t,
		Challenge:  challenge,
		Response:   response,
	}, nil
}

// VerifyDecryptionProof verifies a proof of correct decryption
func VerifyDecryptionProof(C1, C2, m, y, p, g *big.Int, proof *DecryptionProof) bool {
	gS := new(big.Int).Exp(g, proof.Response, p)
	yChallenge := new(big.Int).Exp(y, proof.Challenge, p)
	left := new(big.Int).Mul(gS, yChallenge)

	if left.Cmp(proof.Commitment) != 0 {
		return false
	}

	c1S := new(big.Int).Exp(C1, proof.Response, p)

	c2OverM := new(big.Int).Mul(C2, new(big.Int).ModInverse(m, p))
	c2OverM.Mod(c2OverM, p)

	c2OverMChallenge := new(big.Int).Exp(c2OverM, proof.Challenge, p)

	right := new(big.Int).Mul(c1S, c2OverMChallenge)
	right.Mod(right, p)

	return right.Cmp(proof.Commitment) == 0
}

// ProofOfTallyCorrectness proves that the tally was computed correctly
type TallyProof struct {
	DecryptionProofs []*DecryptionProof
	SumC1            *big.Int
	SumC2            *big.Int
}

// GenerateTallyProof generates proofs for all candidates in the tally
func GenerateTallyProof(encryptedBallots []struct{ C1, C2 *big.Int },
	results []*big.Int, publicKey *struct{ P, G, Y *big.Int }, x *big.Int) (*TallyProof, error) {

	sumC1 := big.NewInt(1)
	sumC2 := big.NewInt(1)

	for _, ballot := range encryptedBallots {
		sumC1.Mul(sumC1, ballot.C1)
		sumC1.Mod(sumC1, publicKey.P)
		sumC2.Mul(sumC2, ballot.C2)
		sumC2.Mod(sumC2, publicKey.P)
	}

	proofs := make([]*DecryptionProof, len(results))
	for i, result := range results {
		proof, err := GenerateDecryptionProof(
			sumC1,
			sumC2,
			result,
			publicKey.Y,
			publicKey.P,
			publicKey.G,
			x,
		)
		if err != nil {
			return nil, err
		}
		proofs[i] = proof
	}

	return &TallyProof{
		DecryptionProofs: proofs,
		SumC1:            sumC1,
		SumC2:            sumC2,
	}, nil
}

func VerifyTallyProof(results []*big.Int, publicKey *struct{ P, G, Y *big.Int }, proof *TallyProof) bool {
	if len(results) != len(proof.DecryptionProofs) {
		return false
	}

	for i, results := range results {
		if !VerifyDecryptionProof(
			proof.SumC1,
			proof.SumC2,
			results,
			publicKey.Y,
			publicKey.P,
			publicKey.G,
			proof.DecryptionProofs[i],
		) {
			return false
		}
	}

	return true
}
