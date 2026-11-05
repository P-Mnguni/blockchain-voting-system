package keyceremony

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/crypto/shamir"
)

// Config controls a single trusted-dealer ceremony.
type Config struct {
	OutputDir        string // where to write shares + public key
	Threshold        int    // e.g. 5
	Total            int    // e.g. 9
	KeySizeBits      int    // e.g. 2048
	PassphrasePrefix string // e.g. "dev-keyholder-" — each share gets prefix + N
}

// Result describes what the ceremony produced.
type Result struct {
	PublicKey    *elgamal.PublicKey
	SharePaths   []string // one per keyholder
	KeyholderIDs []string
	Threshold    int
	Total        int
}

// CeremonyMetadata is written to ceremony.json alongside the shares.
// It records the threshold and total so the tally service knows how many
// shares are required to reconstruct.
type CeremonyMetadata struct {
	Threshold int   `json:"threshold"`
	Total     int   `json:"total"`
	CreatedAt int64 `json:"createdAt"`
}

// RunTrustedDealer generates an ElGamal keypair, splits the private scalar into
// shares, encrypts them, and writes everything to disk.
//
// The private key exists in this process's memory only transiently. After Run
// returns, the caller has no access to it.
func RunTrustedDealer(cfg Config) (*Result, error) {
	if cfg.OutputDir == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	if cfg.Threshold < 2 || cfg.Threshold > cfg.Total {
		return nil, fmt.Errorf("threshold must be in [2, total]")
	}
	if cfg.KeySizeBits == 0 {
		cfg.KeySizeBits = 2048
	}
	if cfg.PassphrasePrefix == "" {
		return nil, fmt.Errorf("passphrase prefix is required")
	}

	// 1. Generate the ElGamal keypair.
	sk, err := elgamal.NewElGamal(cfg.KeySizeBits)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	// 2. Split the private scalar (sk.X) into shares.
	shares, outputLen, err := shamir.SplitScalar(sk.X, cfg.Total, cfg.Threshold)
	if err != nil {
		return nil, fmt.Errorf("split scalar: %w", err)
	}

	// 3. Write shares to disk, encrypted with per-keyholder passphrases.
	if err := os.MkdirAll(cfg.OutputDir, 0o700); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	sharePaths := make([]string, len(shares))
	keyholderIDs := make([]string, len(shares))
	for i, share := range shares {
		id := fmt.Sprintf("keyholder-%d", i+1)
		keyholderIDs[i] = id
		path := filepath.Join(cfg.OutputDir, id+".share")
		sharePaths[i] = path

		passphrase := cfg.PassphrasePrefix + fmt.Sprintf("%d", i+1)
		if err := SaveShare(path, id, passphrase, share, outputLen); err != nil {
			return nil, fmt.Errorf("save share %d: %w", i+1, err)
		}
	}

	// 4. Write the public key to disk (unencrypted, it's public).
	pkJSON, err := marshalPublicKey(sk.PublicKey)
	if err != nil {
		return nil, err
	}
	pkPath := filepath.Join(cfg.OutputDir, "publickey.json")
	if err := os.WriteFile(pkPath, pkJSON, 0o644); err != nil {
		return nil, fmt.Errorf("write public key: %w", err)
	}

	// 5. Wipe the private key from memory (as best Go allows).
	// Zeroing sk.X's internal bytes is not straightforward with big.Int,
	// so we just drop the reference. The GC will collect it eventually.
	// In production, this would use a secure-memory library.
	_ = sk

	// 6. Write ceremony metadata (threshold, total, timestamp)
	meta := CeremonyMetadata{
		Threshold: cfg.Threshold,
		Total:     cfg.Total,
		CreatedAt: time.Now().Unix(),
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal ceremony metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, "ceremony.json"), metaJSON, 0o644); err != nil {
		return nil, fmt.Errorf("write ceremony metadata: %w", err)
	}

	return &Result{
		PublicKey:    &sk.PublicKey,
		SharePaths:   sharePaths,
		KeyholderIDs: keyholderIDs,
		Threshold:    cfg.Threshold,
		Total:        cfg.Total,
	}, nil
}

// marshalPublicKey serializes an ElGamal public key as JSON.
func marshalPublicKey(pk elgamal.PublicKey) ([]byte, error) {
	return json.Marshal(map[string]string{
		"G": pk.G.String(),
		"P": pk.P.String(),
		"Y": pk.Y.String(),
	})
}

// LoadPublicKey reads a public key written by RunTrustedDealer.
func LoadPublicKey(path string) (*elgamal.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	var raw struct{ G, P, Y string }
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	g, ok := newBigInt(raw.G)
	if !ok {
		return nil, fmt.Errorf("invalid G")
	}
	p, ok := newBigInt(raw.P)
	if !ok {
		return nil, fmt.Errorf("invalid P")
	}
	y, ok := newBigInt(raw.Y)
	if !ok {
		return nil, fmt.Errorf("invalid Y")
	}
	return &elgamal.PublicKey{G: g, P: p, Y: y}, nil
}

// ReconstructPrivateKey loads the given shares and reconstructs the original
// ElGamal private key. Used during the tally ceremony.
func ReconstructPrivateKey(sharePaths []string, passphrases []string, outputLen int) (*elgamal.PrivateKey, error) {
	if len(sharePaths) != len(passphrases) {
		return nil, fmt.Errorf("shares and passphrases must have same length")
	}
	if len(sharePaths) == 0 {
		return nil, fmt.Errorf("at least one share is required")
	}

	shares := make([]shamir.Share, len(sharePaths))
	for i, path := range sharePaths {
		share, _, err := LoadShare(path, passphrases[i])
		if err != nil {
			return nil, fmt.Errorf("load share %d: %w", i, err)
		}
		shares[i] = share
	}

	scalar, err := shamir.CombineScalar(shares, outputLen)
	if err != nil {
		return nil, fmt.Errorf("combine scalar: %w", err)
	}

	return &elgamal.PrivateKey{
		PublicKey: elgamal.PublicKey{}, // caller sets this
		X:         scalar,
	}, nil
}

// LoadCeremonyMetadata reads ceremony.json from the given directory.
func LoadCeremonyMetadata(dir string) (*CeremonyMetadata, error) {
	data, err := os.ReadFile(filepath.Join(dir, "ceremony.json"))
	if err != nil {
		return nil, fmt.Errorf("read ceremony metadata: %w", err)
	}
	var meta CeremonyMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse ceremony metadata: %w", err)
	}
	return &meta, nil
}

// helpers

func newBigInt(s string) (*big.Int, bool) {
	if s == "" {
		return nil, false
	}
	return new(big.Int).SetString(s, 10)
}
