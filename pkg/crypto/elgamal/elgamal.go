package elgamal

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// ElGamal encryption with additive homomorphism

type PublicKey struct {
	G *big.Int
	P *big.Int
	Y *big.Int
}

type PrivateKey struct {
	PublicKey
	X *big.Int
}

type Ciphertext struct {
	C1 *big.Int
	C2 *big.Int
}

// NewElGamal generates a new ElGamal key pair
func NewElGamal(bits int) (*PrivateKey, error) {
	p, err := rand.Prime(rand.Reader, bits)
	if err != nil {
		return nil, err
	}

	g := big.NewInt(2)

	x, err := rand.Int(rand.Reader, p)
	if err != nil {
		return nil, err
	}
	x.Add(x, big.NewInt(1))

	y := new(big.Int).Exp(g, x, p)

	return &PrivateKey{
		PublicKey: PublicKey{
			G: g,
			P: p,
			Y: y,
		},
		X: x,
	}, nil
}

// EncodeVote converts a vote value (0 or 1, or any small int) into a group element
func (pk *PublicKey) EncodeVote(vote int64) *big.Int {
	return new(big.Int).Exp(pk.G, big.NewInt(vote), pk.P)
}

// Encrypt encrypts a message (0 or 1 for voting)
func (pk *PublicKey) Encrypt(message *big.Int) (*Ciphertext, error) {
	if message.Sign() <= 0 || message.Cmp(pk.P) >= 0 {
		return nil, errors.New("message must be in [1, p-1] (encode votes as g^vote)")
	}

	r, err := rand.Int(rand.Reader, new(big.Int).Sub(pk.P, big.NewInt(2)))
	if err != nil {
		return nil, err
	}
	r.Add(r, big.NewInt(1))

	c1 := new(big.Int).Exp(pk.G, r, pk.P)

	yR := new(big.Int).Exp(pk.Y, r, pk.P)
	c2 := new(big.Int).Mul(message, yR)
	c2.Mod(c2, pk.P)

	return &Ciphertext{
		C1: c1,
		C2: c2,
	}, nil
}

// EncryptVote is a convenience function that encodes and encrypts a vote
func (pk *PublicKey) EncryptVote(vote int64) (*Ciphertext, error) {
	m := pk.EncodeVote(vote)
	return pk.Encrypt(m)
}

// Decrypt recovers the encoded message (g^vote mod p), NOT the vote itself
func (sk *PrivateKey) Decrypt(ciphertext *Ciphertext) (*big.Int, error) {
	c1X := new(big.Int).Exp(ciphertext.C1, sk.X, sk.P)

	c1XInv := new(big.Int).ModInverse(c1X, sk.P)
	if c1XInv == nil {
		return nil, errors.New("modular inverse failed")
	}

	message := new(big.Int).Mul(ciphertext.C2, c1XInv)
	message.Mod(message, sk.P)

	return message, nil
}

// DecryptVote decrypts and solves the discrete log to recover the vote sum
func (sk *PrivateKey) DecryptVote(ciphertext *Ciphertext, maxSum int64) (int64, error) {
	encoded, err := sk.Decrypt(ciphertext)
	if err != nil {
		return 0, err
	}

	current := big.NewInt(1)
	for i := int64(0); i < maxSum; i++ {
		if current.Cmp(encoded) == 0 {
			return i, nil
		}
		current.Mul(current, sk.G)
		current.Mod(current, sk.P)
	}

	return 0, errors.New("vote sum exceeds maxSum or decryption failed")
}

// Add homomorphically adds two ciphertexts
func Add(c1, c2 *Ciphertext, p *big.Int) *Ciphertext {
	return &Ciphertext{
		C1: new(big.Int).Mod(new(big.Int).Mul(c1.C1, c2.C1), p),
		C2: new(big.Int).Mod(new(big.Int).Mul(c1.C2, c2.C2), p),
	}
}

// AddMany homomorphically adds multiple ciphertexts
func AddMany(ciphertexts []*Ciphertext, p *big.Int) *Ciphertext {
	if len(ciphertexts) == 0 {
		return nil
	}

	result := &Ciphertext{
		C1: new(big.Int).Set(ciphertexts[0].C1),
		C2: new(big.Int).Set(ciphertexts[0].C2),
	}

	for i := 1; i < len(ciphertexts); i++ {
		result.C1.Mul(result.C1, ciphertexts[i].C1)
		result.C1.Mod(result.C1, p)
		result.C2.Mul(result.C2, ciphertexts[i].C2)
		result.C2.Mod(result.C2, p)
	}

	return result
}

// ScalarMul multiplies a ciphertext by a scalar (for weighted voting)
func ScalarMul(c *Ciphertext, scalar int64, p *big.Int) *Ciphertext {
	result := &Ciphertext{
		C1: new(big.Int).Exp(c.C1, big.NewInt(scalar), p),
		C2: new(big.Int).Exp(c.C2, big.NewInt(scalar), p),
	}

	return result
}
