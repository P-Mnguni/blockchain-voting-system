package shamir

import (
	"bytes"
	"testing"
)

func TestSplitAndCombine(t *testing.T) {
	tests := []struct {
		name      string
		secret    []byte
		n         int
		threshold int
	}{
		{
			name:      "small secret, 3-of-5",
			secret:    []byte("my secret key"),
			n:         5,
			threshold: 3,
		},
		{
			name:      "large secret, 5-of-10",
			secret:    []byte("this is a much longer secret that needs to be split across many shares"),
			n:         10,
			threshold: 5,
		},
		{
			name:      "minimum threshold, 2-of-3",
			secret:    []byte("minimal"),
			n:         3,
			threshold: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shares, err := Split(tt.secret, tt.n, tt.threshold)
			if err != nil {
				t.Fatalf("Split failed: %v", err)
			}

			if len(shares) != tt.n {
				t.Errorf("Expected %d shares, got %d", tt.n, len(shares))
			}

			selectedShares := shares[:tt.threshold]
			reconstructed, err := Combine(selectedShares)
			if err != nil {
				t.Fatalf("Combine failed: %v", err)
			}

			if !bytes.Equal(reconstructed, tt.secret) {
				t.Errorf("Reconstructed secret doesn't match original.\nGot: %x\nWant: %x", reconstructed, tt.secret)
			}
		})
	}
}

func TestCannotReconstructWithInsufficientShares(t *testing.T) {
	secret := []byte("top-secret")
	n := 5
	threshold := 3

	shares, err := Split(secret, n, threshold)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	selectedShares := shares[:threshold-1]
	reconstructed, err := Combine(selectedShares)
	if err != nil {
		t.Logf("Combine with insufficient shares returned error (expected): %v", err)
	} else {
		if bytes.Equal(reconstructed, secret) {
			t.Errorf("Reconstructed secret with insufficient shares! This should not happen.")
		}
		t.Logf("Combine with insufficient shares returned incorrect result (expected): %x", reconstructed)
	}
}

func TestAllSharesRequiredForFullReconstruction(t *testing.T) {
	secret := []byte("all-shares-test")
	n := 3
	threshold := 3

	shares, err := Split(secret, n, threshold)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	allShares := shares[:n]
	reconstructed, err := Combine(allShares)
	if err != nil {
		t.Fatalf("Combine failed: %v", err)
	}

	if !bytes.Equal(reconstructed, secret) {
		t.Errorf(("Reconstructed secret doesn't match original"))
	}
}

func TestInvalidParameters(t *testing.T) {
	secret := []byte("test")

	_, err := Split(secret, 3, 1)
	if err == nil {
		t.Error("Expected error for threshold < 2")
	}

	_, err = Split(secret, 2, 3)
	if err == nil {
		t.Error("Expected error for n < threshold")
	}

	_, err = Split([]byte{}, 3, 2)
	if err == nil {
		t.Error("Expected error for empty secret")
	}
}

func TestGF256Arithmetic(t *testing.T) {
	a := byte(0x53)
	b := byte(0xCA)
	result := gf256Mul(a, b)
	expected := byte(0x01)

	if result != expected {
		t.Errorf("Multiplication error: 0x%02X * 0x%02X = 0x%02X, expected 0x%02X", a, b, result, expected)
	}

	a = byte(0x53)
	inv := gf256Inverse(a)
	product := gf256Mul(a, inv)
	if product != 1 {
		t.Errorf("Inverse error: 0x%02X * inv = 0x%02X, expected 0x01", a, product)
	}
}

func BenchmarkSplit(b *testing.B) {
	secret := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		Split(secret, 5, 3)
	}
}

func BenchmarkCombine(b *testing.B) {
	secret := make([]byte, 32)
	shares, _ := Split(secret, 5, 3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Combine(shares[:3])
	}
}
