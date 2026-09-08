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

// Encrypt encrypts a message (0 or 1 for voting)
func (pk *PublicKey) Encrypt(message *big.Int) (*Ciphertext, error) {
	if message.Cmp(big.NewInt(0)) < 0 || message.Cmp(big.NewInt(1)) > 0 {
		return nil, errors.New("message must be 0 or 1 for voting")
	}

	r, err := rand.Int(rand.Reader, pk.P)
	if err != nil {
		return nil, err
	}

	c1 := new(big.Int).Exp(pk.G, r, pk.P)

	yR := new(big.Int).Exp(pk.Y, r, pk.P)
	c2 := new(big.Int).Mul(message, yR)
	c2.Mod(c2, pk.P)

	return &Ciphertext{
		C1: c1,
		C2: c2,
	}, nil
}

// Decrypt decrypts a ciphertext
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

// Add homomorphically adds two ciphertexts
func Add(c1, c2 *Ciphertext) *Ciphertext {
	return &Ciphertext{
		C1: new(big.Int).Mul(c1.C1, c2.C1),
		C2: new(big.Int).Mul(c1.C2, c2.C2),
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

	for i := 0; i < len(ciphertexts); i++ {
		result.C1.Mul(result.C1, ciphertexts[i].C1)
		result.C1.Mod(result.C1, p)
		result.C2.Mul(result.C2, ciphertexts[i].C2)
		result.C2.Mod(result.C2, p)
	}

	return result
}

// ScalarMul multiplies a ciphertext by a scalar (for weighted voting)
func ScalarMul(c *Ciphertext, scalar int, p *big.Int) *Ciphertext {
	result := &Ciphertext{
		C1: new(big.Int).Set(c.C1),
		C2: new(big.Int).Set(c.C2),
	}

	for i := 1; i < scalar; i++ {
		result.C1.Mul(result.C1, c.C1)
		result.C1.Mod(result.C1, p)
		result.C2.Mul(result.C2, c.C2)
		result.C2.Mod(result.C2, p)
	}

	return result
}
