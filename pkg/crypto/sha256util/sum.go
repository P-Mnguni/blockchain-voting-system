package sha256util

import "crypto/sha256"

// Sum returns the raw 32-byte SHA-256 of data.
func Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}
