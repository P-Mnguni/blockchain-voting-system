// Package sha256util is a tiny helper for hex-encoded SHA-256 hashes.
package sha256util

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hex returns the lowercase hex SHA-256 of data.
func Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
