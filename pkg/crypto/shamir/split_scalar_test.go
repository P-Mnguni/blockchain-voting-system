package shamir

import (
	"math/big"
	"testing"
)

func TestSplitScalarRoundTrip(t *testing.T) {
	cases := []string{
		"1",
		"255",
		"65536",
		"123456789012345678901234567890",
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			scalar, _ := new(big.Int).SetString(s, 10)
			shares, outLen, err := SplitScalar(scalar, 5, 3)
			if err != nil {
				t.Fatalf("SplitScalar failed: %v", err)
			}
			if len(shares) != 5 {
				t.Errorf("expected 5 shares, got %d", len(shares))
			}

			// Reconstruct from any 3
			reconstructed, err := CombineScalar(shares[:3], outLen)
			if err != nil {
				t.Fatalf("CombineScalar failed: %v", err)
			}
			if reconstructed.Cmp(scalar) != 0 {
				t.Errorf("mismatch: got %s, want %s", reconstructed, scalar)
			}
		})
	}
}

func TestSplitScalarArbitrarySubsets(t *testing.T) {
	scalar, _ := new(big.Int).SetString("987654321098765432109876543210", 10)
	shares, outLen, err := SplitScalar(scalar, 5, 3)
	if err != nil {
		t.Fatalf("SplitScalar failed: %v", err)
	}

	subsets := [][]int{
		{0, 1, 2}, {0, 1, 3}, {0, 1, 4},
		{0, 2, 3}, {0, 2, 4}, {0, 3, 4},
		{1, 2, 3}, {1, 2, 4}, {1, 3, 4},
		{2, 3, 4},
	}
	for _, subset := range subsets {
		sel := []Share{shares[subset[0]], shares[subset[1]], shares[subset[2]]}
		got, err := CombineScalar(sel, outLen)
		if err != nil {
			t.Errorf("CombineScalar failed for %v: %v", subset, err)
			continue
		}
		if got.Cmp(scalar) != 0 {
			t.Errorf("subset %v produced wrong scalar", subset)
		}
	}
}

func TestSplitScalarInvalidInput(t *testing.T) {
	if _, _, err := SplitScalar(nil, 5, 3); err == nil {
		t.Error("nil scalar should fail")
	}
	neg := big.NewInt(-1)
	if _, _, err := SplitScalar(neg, 5, 3); err == nil {
		t.Error("negative scalar should fail")
	}
}
