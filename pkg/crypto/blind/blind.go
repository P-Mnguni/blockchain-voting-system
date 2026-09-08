package blind

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"math/big"
)

// BlindSignature implements RSA blind signatures
type BlindSignature struct {
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
}

// NewBlindSignature generates a new RSA key pair for blind signatures
func NewBlindSignature(bits int) (*BlindSignature, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}

	return &BlindSignature{
		PublicKey:  &privateKey.PublicKey,
		PrivateKey: privateKey,
	}, nil
}

// BlindMessage blinds a message using a random blinding factor
func (bs *BlindSignature) BlindMessage(message []byte) (*big.Int, *big.Int, error) {
	hash := sha256.Sum256(message)
	m := new(big.Int).SetBytes(hash[:])

	blindFactor, err := rand.Int(rand.Reader, bs.PublicKey.N)
	if err != nil {
		return nil, nil, err
	}

	gcd := new(big.Int).GCD(nil, nil, blindFactor, bs.PublicKey.N)
	for gcd.Cmp(big.NewInt(1)) != 0 {
		blindFactor, err = rand.Int(rand.Reader, bs.PublicKey.N)
		if err != nil {
			return nil, nil, err
		}
		gcd.GCD(nil, nil, blindFactor, bs.PublicKey.N)
	}

	rExpE := new(big.Int).Exp(blindFactor, big.NewInt(int64(bs.PublicKey.E)), bs.PublicKey.N)
	blinded := new(big.Int).Mul(m, rExpE)
	blinded.Mod(blinded, bs.PublicKey.N)

	return blinded, blindFactor, nil
}

// SignBlind signs a blinded message
func (bs *BlindSignature) SignBlind(blinded *big.Int) (*big.Int, error) {
	if bs.PrivateKey == nil {
		return nil, errors.New("private key not available")
	}

	signature := new(big.Int).Exp(blinded, bs.PrivateKey.D, bs.PublicKey.N)
	return signature, nil
}

// UnblindMessage unblinds a signature using the blinding factor
func (bs *BlindSignature) UnblindMessage(signature, blindFactor *big.Int) (*big.Int, error) {
	rInv := new(big.Int).ModInverse(blindFactor, bs.PublicKey.N)
	if rInv == nil {
		return nil, errors.New("blinding factor not invertible")
	}

	unblinded := new(big.Int).Mul(signature, rInv)
	unblinded.Mod(unblinded, bs.PublicKey.N)

	return unblinded, nil
}

// VerifyBlindSignature verifies an unblinded signature
func (bs *BlindSignature) VerifyBlindSignature(message []byte, signature *big.Int) bool {
	hash := sha256.Sum256(message)
	m := new(big.Int).SetBytes(hash[:])

	verification := new(big.Int).Exp(signature, big.NewInt(int64(bs.PublicKey.E)), bs.PublicKey.N)

	return verification.Cmp(m) == 0
}

// BlindSignatureIssuer is a high-level interface for issuing blind signatures
type BlindSignatureIssuer struct {
	bs *BlindSignature
}

// NewBlindSignatureIssuer creates a new issuer
func NewBlindSignatureIssuer(bits int) (*BlindSignatureIssuer, error) {
	bs, err := NewBlindSignature(bits)
	if err != nil {
		return nil, err
	}
	return &BlindSignatureIssuer{bs: bs}, nil
}

// IssueCredential issues a blind signature for a voter's identity
func (issuer *BlindSignatureIssuer) IssueCredential(voterID []byte) ([]byte, error) {
	// hash := sha256.Sum256(voterID)
	// m := new(big.Int).SetBytes(hash[:])

	blinded, blindFactor, err := issuer.bs.BlindMessage(voterID)
	if err != nil {
		return nil, err
	}

	signature, err := issuer.bs.SignBlind(blinded)
	if err != nil {
		return nil, err
	}

	unblinded, err := issuer.bs.UnblindMessage(signature, blindFactor)
	if err != nil {
		return nil, err
	}

	return unblinded.Bytes(), nil
}

// VerifyCredential verifies a blind signature credential
func (issuer *BlindSignatureIssuer) VerifyCredential(voterID []byte, credential []byte) bool {
	signature := new(big.Int).SetBytes(credential)
	return issuer.bs.VerifyBlindSignature(voterID, signature)
}
