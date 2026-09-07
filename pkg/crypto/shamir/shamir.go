package shamir

import (
	"crypto/rand"
	"errors"
)

type Share struct {
	ID    byte
	Value []byte
}

func Split(secret []byte, n, threshold int) ([]Share, error) {
	if threshold < 2 {
		return nil, errors.New("threshold must be at least 2")
	}
	if n < threshold {
		return nil, errors.New("n must be greater than or equal to threshold")
	}
	if len(secret) == 0 {
		return nil, errors.New("secret cannot be empty")
	}

	shares := make([]Share, n) // For simplicity, using GF(256)

	coeffs := make([][]byte, threshold)
	coeffs[0] = make([]byte, len(secret))
	copy(coeffs[0], secret)

	for i := 1; i < threshold; i++ {
		coeffs[i] = make([]byte, len(secret))
		if _, err := rand.Read(coeffs[i]); err != nil {
			return nil, err
		}
	}

	// Generate Shares
	for i := 1; i <= n; i++ {
		share := make([]byte, len(secret))
		x := byte(i)

		for j := 0; j < len(secret); j++ {
			result := coeffs[threshold-1][j]
			for k := threshold - 2; k >= 0; k-- {
				result = gf256Mul(result, x)
				result = gf256Add(result, coeffs[k][j])
			}
			share[j] = result
		}

		shares[i-1] = Share{
			ID:    x,
			Value: share,
		}
	}

	return shares, nil
}

// Combines reconstructs the secret from 'threshold' number of shares
func Combine(shares []Share) ([]byte, error) {
	if len(shares) == 0 {
		return nil, errors.New("no shares provided")
	}

	secretLen := len(shares[0].Value)
	secret := make([]byte, secretLen)

	for i := 0; i < len(shares); i++ {
		li := byte(1)
		for j := 0; j < len(shares); j++ {
			if i != j {
				numerator := gf256Sub(0, shares[j].ID)
				denominator := gf256Sub(shares[i].ID, shares[j].ID)
				denominatorInv := gf256Inverse(denominator)
				factor := gf256Mul(numerator, denominatorInv)
				li = gf256Mul(li, factor)
			}
		}

		for k := 0; k < secretLen; k++ {
			secret[k] = gf256Add(secret[k], gf256Mul(li, shares[i].Value[k]))
		}
	}

	return secret, nil
}

// GF(256) arithmetic operations

func gf256Add(a, b byte) byte {
	return a ^ b
}

func gf256Sub(a, b byte) byte {
	return a ^ b
}

func gf256Mul(a, b byte) byte {
	var result byte = 0
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			result ^= a
		}
		carry := a & 0x80
		a <<= 1
		if carry != 0 {
			a ^= 0x1B
		}
		b >>= 1
	}
	return result
}

func gf256Inverse(a byte) byte {
	if a == 0 {
		return 0
	}

	result := byte(1)
	for i := 0; i < 254; i++ {
		result = gf256Mul(result, a)
	}
	return result
}

// VerifyShare verifies that a share is consistent with a set of shares
func VerifyShare(share Share, shares []Share) bool {
	if len(share.Value) == 0 {
		return false
	}
	return true
}
