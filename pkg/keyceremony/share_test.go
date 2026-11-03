package keyceremony

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto/shamir"
)

// --- Happy path ---

func TestSaveLoadShareRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.share")
	share := shamir.Share{ID: 3, Value: []byte{1, 2, 3, 4, 5}}

	if err := SaveShare(path, "keyholder-3", "correct horse", share, 6); err != nil {
		t.Fatalf("SaveShare failed: %v", err)
	}

	loaded, outLen, err := LoadShare(path, "correct horse")
	if err != nil {
		t.Fatalf("LoadShare failed: %v", err)
	}
	if loaded.ID != share.ID {
		t.Errorf("share ID mismatch: got %d, want %d", loaded.ID, share.ID)
	}
	if string(loaded.Value) != string(share.Value) {
		t.Errorf("share value mismatch: got %x, want %x", loaded.Value, share.Value)
	}
	if outLen != 6 {
		t.Errorf("outputLen mismatch: got %d, want 6", outLen)
	}
}

func TestSaveShareCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "test.share")
	share := shamir.Share{ID: 1, Value: []byte{42}}

	if err := SaveShare(nested, "keyholder-1", "pass", share, 2); err != nil {
		t.Fatalf("SaveShare failed: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("share file not created at nested path: %v", err)
	}
}

func TestSaveShareOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.share")

	first := shamir.Share{ID: 1, Value: []byte{1}}
	if err := SaveShare(path, "kh-1", "pass", first, 2); err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	second := shamir.Share{ID: 2, Value: []byte{2, 3}}
	if err := SaveShare(path, "kh-2", "pass", second, 3); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	loaded, outLen, err := LoadShare(path, "pass")
	if err != nil {
		t.Fatalf("LoadShare failed: %v", err)
	}
	if loaded.ID != second.ID {
		t.Errorf("overwrite didn't take effect: got ID %d", loaded.ID)
	}
	if outLen != 3 {
		t.Errorf("overwrite didn't update outputLen: got %d", outLen)
	}
}

// --- Failure modes ---

func TestLoadShareWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.share")
	share := shamir.Share{ID: 1, Value: []byte{9, 9, 9}}

	if err := SaveShare(path, "keyholder-1", "correct", share, 4); err != nil {
		t.Fatalf("SaveShare failed: %v", err)
	}

	_, _, err := LoadShare(path, "wrong")
	if err == nil {
		t.Fatal("expected error with wrong passphrase")
	}
	// AES-GCM decryption failure; error message should hint at passphrase.
	if !strings.Contains(err.Error(), "passphrase") && !strings.Contains(err.Error(), "decrypt") {
		t.Logf("error message: %v", err)
	}
}

func TestLoadShareMissingFile(t *testing.T) {
	_, _, err := LoadShare("/nonexistent/path/to/share", "pass")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadShareCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.share")

	if err := os.WriteFile(path, []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, _, err := LoadShare(path, "any")
	if err == nil {
		t.Fatal("expected error with corrupted JSON")
	}
	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "record") {
		t.Logf("error message: %v", err)
	}
}

func TestLoadShareEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.share")

	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, _, err := LoadShare(path, "any")
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestLoadShareUnsupportedKDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.share")

	record := ShareRecord{
		KeyholderID: "kh-1",
		KDF:         "argon2id", // not what we support
		Iterations:  1,
		Salt:        make([]byte, 16),
		Nonce:       make([]byte, 12),
		Ciphertext:  []byte{0, 1, 2},
	}
	data, _ := json.Marshal(record)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, _, err := LoadShare(path, "any")
	if err == nil {
		t.Fatal("expected error for unsupported KDF")
	}
	if !strings.Contains(err.Error(), "KDF") {
		t.Errorf("error should mention KDF, got: %v", err)
	}
}

func TestLoadShareTamperedCiphertext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.share")
	share := shamir.Share{ID: 5, Value: []byte{1, 2, 3}}

	if err := SaveShare(path, "kh-5", "pass", share, 4); err != nil {
		t.Fatalf("SaveShare failed: %v", err)
	}

	// Read, tamper with ciphertext, write back.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var record ShareRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	record.Ciphertext[0] ^= 0xFF // flip a bit
	tampered, _ := json.Marshal(record)
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, _, err = LoadShare(path, "pass")
	if err == nil {
		t.Fatal("expected error for tampered ciphertext (GCM auth tag should fail)")
	}
}

// --- File permission checks ---

func TestSaveShareFilePermissions(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping permission check in CI (running as root sometimes bypasses)")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.share")
	share := shamir.Share{ID: 1, Value: []byte{1, 2, 3}}

	if err := SaveShare(path, "kh-1", "pass", share, 4); err != nil {
		t.Fatalf("SaveShare failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	// Should be readable/writable by owner only.
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		t.Errorf("share file has overly permissive mode: %o (expected 0600)", mode)
	}
}

// --- Encryption format checks ---

func TestSavedShareIsEncrypted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.share")

	plainShareValue := []byte("this-is-the-secret-value")
	share := shamir.Share{ID: 7, Value: plainShareValue}

	if err := SaveShare(path, "kh-7", "pass", share, 32); err != nil {
		t.Fatalf("SaveShare failed: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// The raw share bytes should NOT appear in the file.
	if strings.Contains(string(raw), string(plainShareValue)) {
		t.Error("plaintext share value found in encrypted file — encryption is broken")
	}
}

func TestDifferentSaltEachSave(t *testing.T) {
	dir := t.TempDir()
	share := shamir.Share{ID: 1, Value: []byte{1, 2, 3}}

	path1 := filepath.Join(dir, "s1.share")
	path2 := filepath.Join(dir, "s2.share")

	if err := SaveShare(path1, "kh-1", "samepass", share, 4); err != nil {
		t.Fatalf("save 1 failed: %v", err)
	}
	if err := SaveShare(path2, "kh-1", "samepass", share, 4); err != nil {
		t.Fatalf("save 2 failed: %v", err)
	}

	d1, _ := os.ReadFile(path1)
	d2, _ := os.ReadFile(path2)

	var r1, r2 ShareRecord
	_ = json.Unmarshal(d1, &r1)
	_ = json.Unmarshal(d2, &r2)

	if string(r1.Salt) == string(r2.Salt) {
		t.Error("salt is identical across two saves — randomness is broken")
	}
	if string(r1.Ciphertext) == string(r2.Ciphertext) {
		t.Error("ciphertext is identical across two saves — nonce reuse or broken encryption")
	}
}

// --- Round-trip of the record structure ---

func TestShareRecordJSONRoundTrip(t *testing.T) {
	record := ShareRecord{
		KeyholderID: "keyholder-1",
		KDF:         "pbkdf2-sha256",
		Iterations:  100000,
		Salt:        []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Nonce:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		Ciphertext:  []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var back ShareRecord
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if back.KeyholderID != record.KeyholderID {
		t.Error("KeyholderID mismatch")
	}
	if back.KDF != record.KDF {
		t.Error("KDF mismatch")
	}
	if back.Iterations != record.Iterations {
		t.Error("Iterations mismatch")
	}
	if string(back.Salt) != string(record.Salt) {
		t.Error("Salt mismatch")
	}
	if string(back.Nonce) != string(record.Nonce) {
		t.Error("Nonce mismatch")
	}
	if string(back.Ciphertext) != string(record.Ciphertext) {
		t.Error("Ciphertext mismatch")
	}
}
