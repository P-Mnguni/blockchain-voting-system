package keyceremony

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"crypto/pbkdf2"

	"github.com/blockchain-voting-system/pkg/crypto/shamir"
)

// ShareRecord is the on-disk representation of an encrypted keyholder share.
type ShareRecord struct {
	KeyholderID string `json:"keyholderId"` // e.g. "keyholder-1"
	KDF         string `json:"kdf"`         // "pbkdf2-sha256"
	Iterations  int    `json:"iterations"`  // e.g. 100000
	Salt        []byte `json:"salt"`        // 16 bytes
	Nonce       []byte `json:"nonce"`       // 12 bytes (AES-GCM)
	Ciphertext  []byte `json:"ciphertext"`  // encrypted share payload
}

// sharePlaintext is what we actually encrypt.
type sharePlaintext struct {
	ShareID    byte   `json:"shareId"`
	ShareValue []byte `json:"shareValue"`
	OutputLen  int    `json:"outputLen"`
}

// SaveShare encrypts and writes a share to disk.
func SaveShare(path, keyholderID, passphrase string, share shamir.Share, outputLen int) error {
	plaintext := sharePlaintext{
		ShareID:    share.ID,
		ShareValue: share.Value,
		OutputLen:  outputLen,
	}
	plainBytes, err := json.Marshal(plaintext)
	if err != nil {
		return fmt.Errorf("marshal share: %w", err)
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	const iterations = 100000
	// Note: hash.New comes FIRST, password is a string, and it returns an error.
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, iterations, 32)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plainBytes, nil)

	record := ShareRecord{
		KeyholderID: keyholderID,
		KDF:         "pbkdf2-sha256",
		Iterations:  iterations,
		Salt:        salt,
		Nonce:       nonce,
		Ciphertext:  ciphertext,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create share directory: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write share: %w", err)
	}
	return nil
}

// LoadShare reads and decrypts a share from disk.
func LoadShare(path, passphrase string) (shamir.Share, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return shamir.Share{}, 0, fmt.Errorf("read share file: %w", err)
	}
	var record ShareRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return shamir.Share{}, 0, fmt.Errorf("parse share record: %w", err)
	}

	if record.KDF != "pbkdf2-sha256" {
		return shamir.Share{}, 0, fmt.Errorf("unsupported KDF: %s", record.KDF)
	}

	key, err := pbkdf2.Key(sha256.New, passphrase, record.Salt, record.Iterations, 32)
	if err != nil {
		return shamir.Share{}, 0, fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return shamir.Share{}, 0, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return shamir.Share{}, 0, fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, nil)
	if err != nil {
		return shamir.Share{}, 0, fmt.Errorf("decrypt share (wrong passphrase?): %w", err)
	}

	var plain sharePlaintext
	if err := json.Unmarshal(plaintext, &plain); err != nil {
		return shamir.Share{}, 0, fmt.Errorf("parse share payload: %w", err)
	}

	return shamir.Share{ID: plain.ShareID, Value: plain.ShareValue}, plain.OutputLen, nil
}
