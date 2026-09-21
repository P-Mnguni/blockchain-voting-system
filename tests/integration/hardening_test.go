package integration

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/blockchain-voting-system/pkg/crypto"
)

// SHAMIR: SUBSET INDEPENDENCE

/*
TestShamirArbitrarySubsets verifies that ANY subset of 'threshold' shares
(not just the first N) reconstructs the secret.
*/
func TestShamirArbitrarySubsets(t *testing.T) {
	secret := []byte("threshold-elections-need-subset-independence")
	n := 5
	threshold := 3

	shares, err := crypto.SplitShamir(secret, n, threshold)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	subsets := [][]int{
		{0, 1, 2}, {0, 1, 3}, {0, 1, 4},
		{0, 2, 3}, {0, 2, 4}, {0, 3, 4},
		{1, 2, 3}, {1, 2, 4}, {1, 3, 4},
		{2, 3, 4},
	}

	for _, subset := range subsets {
		selected := []crypto.Share{shares[subset[0]], shares[subset[1]], shares[subset[2]]}
		reconstructed, err := crypto.CombineShamir(selected)
		if err != nil {
			t.Errorf("Combine failed for subset %v: %v", subset, err)
			continue
		}
		if !bytes.Equal(reconstructed, secret) {
			t.Errorf("Subset %v produced wrong secret", subset)
		}
	}
}

/*
TestShamirSharesAreDistinct ensures shares are actually different from
each other (a trivial bug would produce identical shares).
*/
func TestShamirSharesAreDistinct(t *testing.T) {
	secret := []byte("distinct-shares-please")
	shares, err := crypto.SplitShamir(secret, 5, 3)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	for i := 0; i < len(shares); i++ {
		for j := i + 1; j < len(shares); j++ {
			if bytes.Equal(shares[i].Value, shares[j].Value) {
				t.Errorf("Shares %d and %d are identical", i, j)
			}
			if shares[i].ID == shares[j].ID {
				t.Errorf("Shares %d and %d have the same ID", i, j)
			}
		}
	}
}

/*
TestShamirShareIDsAreNonZero documents that share IDs start at 1
(ID 0 would break Lagrange interpolation since we evaluate at x=0)
*/
func TestShamirShareIDsAreNonZero(t *testing.T) {
	shares, err := crypto.SplitShamir([]byte("no-zero-ids"), 5, 3)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	for i, s := range shares {
		if s.ID == 0 {
			t.Errorf("Share %d has ID 0, which breaks Lagrange interpolation", i)
		}
	}
}

// ELGAMAL: EDGE CASES

/*
TestElGamalAddManyEmpty ensure AddMany surfaces a clear error on empty input
rather than returning nil (which would crash callers in the chaincode)
*/
func TestElGamalAddManyEmpty(t *testing.T) {
	sk, err := crypto.NewElGamal(2048)
	if err != nil {
		t.Fatalf("NewElGamal failed: %v", err)
	}

	result, err := crypto.ElGamalAddMany([]*crypto.Ciphertext{}, sk.P)
	if err == nil {
		t.Fatal("AddMany with empty slice should return an error")
	}
	if result != nil {
		t.Error("AddMany with empty slice should return nil result")
	}
}

// TestElGamalAddManyNil ensures AddMany rejects nil ciphertexts inside the slice.
func TestElGamalAddManyNil(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)

	_, err := crypto.ElGamalAddMany([]*crypto.Ciphertext{ct, nil, ct}, sk.P)
	if err == nil {
		t.Error("AddMany with nil element should return an error")
	}
}

/*
TestElGamalDecryptVoteMaxSumTooLow ensures DecryptVote returns a clear
error when the sum exceeds the caller-provided bound.
*/
func TestElGamalDecryptVoteMaxSumTooLow(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)

	ciphertexts := make([]*crypto.Ciphertext, 100)
	for i := range ciphertexts {
		ct, err := sk.EncryptVote(1)
		if err != nil {
			t.Fatalf("EncryptVote failed: %v", err)
		}
		ciphertexts[i] = ct
	}

	sum, err := crypto.ElGamalAddMany(ciphertexts, sk.P)
	if err != nil {
		t.Fatalf("AddMany failed: %v", err)
	}

	_, err = sk.DecryptVote(sum, 50)
	if err == nil {
		t.Error("DecryptVote should fail when maxSum < actual sum")
	}

	result, err := sk.DecryptVote(sum, 100)
	if err != nil {
		t.Fatalf("DecryptVote with sufficient maxSum failed: %v", err)
	}
	if result != 100 {
		t.Errorf("Expected 100, got %d", result)
	}
}

// TestElGamalEncryptVoteNegative ensures negative votes are rejected loudly
func TestElGamalEncryptVoteNegative(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)
	_, err := sk.EncryptVote(-1)
	if err == nil {
		t.Error("EncryptVote(-1) should return an error")
	}
}

/*
TestElGamalAddOrderIndependence ensures homomorphic addition is commutative
and associative - the tally result must not depend on the order ballots
*/
func TestElGamalAddOrderIndependence(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)

	votes := []int64{1, 0, 1, 1, 0, 1, 0, 0, 1, 1}
	cts := make([]*crypto.Ciphertext, len(votes))
	for i, v := range votes {
		ct, err := sk.EncryptVote(v)
		if err != nil {
			t.Fatalf("EncryptVote failed: %v", err)
		}
		cts[i] = ct
	}

	sumFwd, err := crypto.ElGamalAddMany(cts, sk.P)
	if err != nil {
		t.Fatalf("AddMany forward failed: %v", err)
	}
	expected := int64(0)
	for _, v := range votes {
		expected += v
	}

	reversed := make([]*crypto.Ciphertext, len(cts))
	for i := range cts {
		reversed[i] = cts[len(cts)-1-i]
	}
	sumRev, err := crypto.ElGamalAddMany(reversed, sk.P)
	if err != nil {
		t.Fatalf("AddMany reversed failed: %v", err)
	}

	resFwd, err := sk.DecryptVote(sumFwd, 100)
	if err != nil {
		t.Fatalf("DecryptVote forward failed: %v", err)
	}
	resRev, err := sk.DecryptVote(sumRev, 100)
	if err != nil {
		t.Fatalf("DecryptVOte reversed failed: %v", err)
	}

	if resFwd != expected {
		t.Errorf("Forward sum: got %d, want %d", resFwd, expected)
	}
	if resRev != expected {
		t.Errorf("Reverse sum: got %d, want %d", resRev, expected)
	}
}

// SERIALIZATION: THE HIGHEST-RISK GAP FOR MILESTONE 2

/*
TestCiphertextJSONRoundTrip verifies that a Ciphertext survives JSON
serialization. The chaincode will marshal ciphertexts over the wire
*/
func TestCiphertextJSONRoundTrip(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)

	blob, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var ctBack crypto.Ciphertext
	if err := json.Unmarshal(blob, &ctBack); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	resOrig, err := sk.DecryptVote(ct, 10)
	if err != nil {
		t.Fatalf("DecryptVote original failed: %v", err)
	}
	resBack, err := sk.DecryptVote(&ctBack, 10)
	if err != nil {
		t.Fatalf("DecryptVote after round-trip failed: %v", err)
	}
	if resOrig != resBack {
		t.Errorf("Round-trip changed the value: %d vs %d", resOrig, resBack)
	}
}

/*
TestCiphertextJSONRoundTripHomomorphic verifies that homomorphic addition
still works after the operands have been serialized and deserialized
*/
func TestCiphertextJSONRoundTripHomomorphic(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)

	ct1, _ := sk.EncryptVote(1)
	ct2, _ := sk.EncryptVote(1)

	blob1, _ := json.Marshal(ct1)
	blob2, _ := json.Marshal(ct2)
	var ct1Back, ct2Back crypto.Ciphertext
	json.Unmarshal(blob1, &ct1Back)
	json.Unmarshal(blob2, &ct2Back)

	sum, err := crypto.ElGamalAddMany([]*crypto.Ciphertext{&ct1Back, &ct2Back}, sk.P)
	if err != nil {
		t.Fatalf("AddMany failed: %v", err)
	}
	result, err := sk.DecryptVote(sum, 10)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}
	if result != 2 {
		t.Errorf("Expected 2 after round-trip, got %d", result)
	}
}

// BLIND SIGNATURE: FORGERY RESISTANCE

/*
TestBlindSignatureForgeryResistance verifies that a tampered signature
fails verification. Core security property of signature scheme
*/
func TestBlindSignatureForgeryResistance(t *testing.T) {
	issuer, err := crypto.NewBlindIssuer(2048)
	if err != nil {
		t.Fatalf("NewBlindSignatureIssuer failed: %v", err)
	}

	voterID := []byte("voter-12345")
	cred, err := issuer.IssueCredential(voterID)
	if err != nil {
		t.Fatalf("IssueCredential failed: %v", err)
	}

	if !issuer.VerifyCredential(voterID, cred) {
		t.Fatalf("Valid credential failed verification")
	}

	tampered := make([]byte, len(cred))
	copy(tampered, cred)
	tampered[len(tampered)-1] ^= 0x01

	if issuer.VerifyCredential(voterID, tampered) {
		t.Error("Tampered signature passed verification - forgery resistance broken")
	}

	if len(cred) > 1 {
		truncated := cred[:len(cred)-1]
		if issuer.VerifyCredential(voterID, truncated) {
			t.Error("Truncate signature passed verification")
		}
	}

	if issuer.VerifyCredential(voterID, []byte{}) {
		t.Error("Empty credential passed verification")
	}
}

// TestBlindSignatureWrongVoterID verifies the credential is bound to the voter
func TestBlindSignatureWrongVoterID(t *testing.T) {
	issuer, _ := crypto.NewBlindIssuer(2048)

	voterID := []byte("voter-A")
	cred, _ := issuer.IssueCredential(voterID)

	if issuer.VerifyCredential([]byte("voter-B"), cred) {
		t.Error("Credential verified for wrong voter ID")
	}
}

// ZK PROOFS: SOUNDNESS UNDER TAMPERING

/*
TestDecryptionProofTamperResistance verifies that mutating any component
of the proof causes verification to fail. Soundness property
*/
func TestDecryptionProofTamperResistance(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)
	m := new(big.Int).Exp(sk.G, big.NewInt(1), sk.P)

	proof, err := crypto.GenerateDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, sk.X)
	if err != nil {
		t.Fatalf("GenerateDecryptionProof failed: %v", err)
	}

	if !crypto.VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Fatal("Valid proof failed verification")
	}

	origA := new(big.Int).Set(proof.A)
	proof.A.Add(proof.A, big.NewInt(1))
	if crypto.VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Error("Proof with tampered A verified")
	}
	proof.A = origA

	origB := new(big.Int).Set(proof.B)
	proof.B.Add(proof.B, big.NewInt(1))
	if crypto.VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Error("Proof with tampered B verified")
	}
	proof.B = origB

	origS := new(big.Int).Set(proof.S)
	proof.S.Add(proof.S, big.NewInt(1))
	if crypto.VerifyDecryptionProof(ct.C1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Error("Proof with tampered S verified")
	}
	proof.S = origS

	tamperedC1 := new(big.Int).Add(ct.C1, big.NewInt(1))
	if crypto.VerifyDecryptionProof(tamperedC1, ct.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Error("Proof verified against tampered ciphertext")
	}
}

// TestDecryptionProofWrongPlaintext verifies soundness against a false claim.
func TestDecryptionProofWrongPlaintext(t *testing.T) {
	sk, _ := crypto.NewElGamal(2048)
	ct, _ := sk.EncryptVote(1)

	mReal := new(big.Int).Exp(sk.G, big.NewInt(1), sk.P)
	proof, err := crypto.GenerateDecryptionProof(ct.C1, ct.C2, mReal, sk.Y, sk.P, sk.G, sk.X)
	if err != nil {
		t.Fatalf("GenerateDecryptionProof failed: %v", err)
	}

	mFake := new(big.Int).Exp(sk.G, big.NewInt(0), sk.P)
	if crypto.VerifyDecryptionProof(ct.C1, ct.C2, mFake, sk.Y, sk.P, sk.G, proof) {
		t.Error("Proof verified for a plaintext that was not claimed")
	}
}

// CROSS-PRIMITIVE: FULL END-TO-END PATH WITH SERIALIZATION

/*
TestFullPipelineWithSerialization walks through the entire crypto pipeline
as Milestone 2 will actually use it:
register → sign → encrypt → serialization → sum → decrypt → prove → verify
*/
func TestFullPipelineWithSerialization(t *testing.T) {
	issuer, err := crypto.NewBlindIssuer(2048)
	if err != nil {
		t.Fatalf("NewBlindSignatureIssuer failed: %v", err)
	}
	sk, err := crypto.NewElGamal(2048)
	if err != nil {
		t.Fatalf("NewElGamal failed: %v", err)
	}

	keyBytes := sk.X.Bytes()
	shares, err := crypto.SplitShamir(keyBytes, 9, 5)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	voterID := []byte("voter-001")
	cred, err := issuer.IssueCredential(voterID)
	if err != nil {
		t.Fatalf("IssueCredential failed: %v", err)
	}
	if !issuer.VerifyCredential(voterID, cred) {
		t.Fatal("Credential verification failed")
	}

	ct, err := sk.EncryptVote(1)
	if err != nil {
		t.Fatalf("EncryptVote failed: %v", err)
	}
	blob, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	ct2, _ := sk.EncryptVote(1)
	blob2, _ := json.Marshal(ct2)

	var c1, c2 crypto.Ciphertext
	if err := json.Unmarshal(blob, &c1); err != nil {
		t.Fatalf("Unmarshal c1 failed: %v", err)
	}
	if err := json.Unmarshal(blob2, &c2); err != nil {
		t.Fatalf("Unmarshal c2 failed: %v", err)
	}

	sum, err := crypto.ElGamalAddMany([]*crypto.Ciphertext{&c1, &c2}, sk.P)
	if err != nil {
		t.Fatalf("AddMany failed: %v", err)
	}

	reconstructed, err := crypto.CombineShamir(shares[:5])
	if err != nil {
		t.Fatalf("Combine failed: %v", err)
	}
	reconstructedX := new(big.Int).SetBytes(reconstructed)
	if reconstructedX.Cmp(sk.X) != 0 {
		t.Fatal("Reconstructed private key does not match original")
	}

	result, err := sk.DecryptVote(sum, 10)
	if err != nil {
		t.Fatalf("DecryptVote failed: %v", err)
	}
	if result != 2 {
		t.Errorf("Expected tally 2, got %d", result)
	}

	m := new(big.Int).Exp(sk.G, big.NewInt(result), sk.P)
	proof, err := crypto.GenerateDecryptionProof(sum.C1, sum.C2, m, sk.Y, sk.P, sk.G, sk.X)
	if err != nil {
		t.Fatalf("GenerateDecryptionProof failed: %v", err)
	}

	if !crypto.VerifyDecryptionProof(sum.C1, sum.C2, m, sk.Y, sk.P, sk.G, proof) {
		t.Error("End-to-end proof verification failed")
	}
}
