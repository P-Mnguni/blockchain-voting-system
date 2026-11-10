package shamir

import (
	"fmt"
	"math/big"
)

/*
SplitScalar splits a big.Int into shares over GF(256) by first encoding the
scalar into fixed-length big-endian bytes.

The scalar must be non-negative. Shares reconstruct the exact same scalar.
*/
func SplitScalar(scalar *big.Int, n, threshold int) ([]Share, int, error) {
	if scalar == nil {
		return nil, 0, fmt.Errorf("scalar is nil")
	}
	if scalar.Sign() < 0 {
		return nil, 0, fmt.Errorf("scalar must be non-negative")
	}

	// Pad to a fixed length so every share has the same size.
	raw := scalar.Bytes()
	if len(raw) == 0 {
		raw = []byte{0}
	}
	padded := make([]byte, len(raw)+1)
	copy(padded[1:], raw)

	shares, err := Split(padded, n, threshold)
	if err != nil {
		return nil, 0, err
	}
	return shares, len(padded), nil
}

// CombineScalar reconstructs a big.Int from shares produced by SplitScalar.
func CombineScalar(shares []Share, outputLen int) (*big.Int, error) {
	combined, err := Combine(shares)
	if err != nil {
		return nil, err
	}
	if len(combined) != outputLen {
		return nil, fmt.Errorf("combined length %d, expected %d", len(combined), outputLen)
	}
	// Strip the leading zero pad byte.
	scalar := new(big.Int).SetBytes(combined[1:])
	return scalar, nil
}
