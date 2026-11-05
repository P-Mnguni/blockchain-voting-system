package keyceremony

import (
	"os"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
)

func TestRunTrustedDealer(t *testing.T) {
	dir := t.TempDir()

	result, err := RunTrustedDealer(Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024, // smaller for speed
		PassphrasePrefix: "test-pass-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	if len(result.SharePaths) != 5 {
		t.Errorf("expected 5 shares, got %d", len(result.SharePaths))
	}
	if result.Threshold != 3 || result.Total != 5 {
		t.Errorf("threshold/total mismatch: %d/%d", result.Threshold, result.Total)
	}

	// Public key file should exist.
	pk, err := LoadPublicKey(dir + "/publickey.json")
	if err != nil {
		t.Fatalf("LoadPublicKey failed: %v", err)
	}
	if pk.P.Cmp(result.PublicKey.P) != 0 {
		t.Error("public key mismatch")
	}

	// Every share file should exist.
	for _, path := range result.SharePaths {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("share file missing: %s", path)
		}
	}
}

func TestReconstructPrivateKey(t *testing.T) {
	dir := t.TempDir()
	result, err := RunTrustedDealer(Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024,
		PassphrasePrefix: "test-pass-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	// Reconstruct from first 3 shares.
	paths := result.SharePaths[:3]
	passes := []string{"test-pass-1", "test-pass-2", "test-pass-3"}

	// Read the outputLen from one of the shares.
	_, outLen, err := LoadShare(paths[0], passes[0])
	if err != nil {
		t.Fatalf("LoadShare failed: %v", err)
	}

	sk, err := ReconstructPrivateKey(paths, passes, outLen)
	if err != nil {
		t.Fatalf("ReconstructPrivateKey failed: %v", err)
	}

	// Verify the reconstructed key works by encrypting + decrypting.
	ct, err := result.PublicKey.EncryptVote(1)
	if err != nil {
		t.Fatalf("EncryptVote failed: %v", err)
	}

	sk.PublicKey = *result.PublicKey
	plain, err := sk.DecryptVote(ct, 10)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}
	if plain != 1 {
		t.Errorf("expected 1, got %d", plain)
	}
}

func TestReconstructWithWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	result, err := RunTrustedDealer(Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024,
		PassphrasePrefix: "test-pass-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	paths := result.SharePaths[:3]
	wrongPasses := []string{"wrong", "wrong", "wrong"}

	_, outLen, _ := LoadShare(paths[0], "test-pass-1")
	_, err = ReconstructPrivateKey(paths, wrongPasses, outLen)
	if err == nil {
		t.Fatal("expected decryption failure with wrong passphrase")
	}
}

func TestReconstructWithInsufficientShares(t *testing.T) {
	dir := t.TempDir()
	result, err := RunTrustedDealer(Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024,
		PassphrasePrefix: "test-pass-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	// Only 2 shares — below threshold
	paths := result.SharePaths[:2]
	passes := []string{"test-pass-1", "test-pass-2"}
	_, outLen, _ := LoadShare(paths[0], passes[0])

	sk, err := ReconstructPrivateKey(paths, passes, outLen)
	if err != nil {
		// Error is fine
		return
	}

	// If it didn't error, the key must be wrong (encryption/decryption round-trip fails)
	ct, _ := result.PublicKey.EncryptVote(1)
	sk.PublicKey = *result.PublicKey
	plain, _ := sk.DecryptVote(ct, 100)
	if plain == 1 {
		t.Error("reconstruction with insufficient shares produced the correct key — Shamir is broken")
	}
}

func TestRunTrustedDealerInvalidConfig(t *testing.T) {
	if _, err := RunTrustedDealer(Config{Threshold: 1, Total: 5, PassphrasePrefix: "x"}); err == nil {
		t.Error("threshold=1 should fail")
	}
	if _, err := RunTrustedDealer(Config{Threshold: 5, Total: 3, PassphrasePrefix: "x"}); err == nil {
		t.Error("threshold>total should fail")
	}
	if _, err := RunTrustedDealer(Config{Threshold: 3, Total: 5}); err == nil {
		t.Error("missing OutputDir should fail")
	}
}

func TestCeremonyMetadataWritten(t *testing.T) {
	dir := t.TempDir()
	_, err := RunTrustedDealer(Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024,
		PassphrasePrefix: "test-pass-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	meta, err := LoadCeremonyMetadata(dir)
	if err != nil {
		t.Fatalf("LoadCeremonyMetadata failed: %v", err)
	}
	if meta.Threshold != 3 {
		t.Errorf("threshold: got %d, want 3", meta.Threshold)
	}
	if meta.Total != 5 {
		t.Errorf("total: got %d, want 5", meta.Total)
	}
	if meta.CreatedAt == 0 {
		t.Error("createdAt should be set")
	}
}

// Silence unused-import warnings if elgamal isn't used elsewhere.
var _ = elgamal.PublicKey{}
