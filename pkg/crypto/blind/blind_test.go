package blind

import (
	"bytes"
	"testing"
)

func TestBlindSignature(t *testing.T) {
	bs, err := NewBlindSignature(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	message := []byte("voter-12345-credential")

	blinded, blindFactor, err := bs.BlindMessage(message)
	if err != nil {
		t.Fatalf("BlindMessage failed: %v", err)
	}

	signature, err := bs.SignBlind(blinded)
	if err != nil {
		t.Fatalf("SignBlind failed %v", err)
	}

	unblinded, err := bs.UnblindMessage(signature, blindFactor)
	if err != nil {
		t.Fatalf("UnblindMessage failed: %v", err)
	}

	if !bs.VerifyBlindSignature(message, unblinded) {
		t.Error("Signature verification failed")
	}

	wrongMessage := []byte("wrong-message")
	if bs.VerifyBlindSignature(wrongMessage, unblinded) {
		t.Error("Signature verification should fail for wrong message")
	}
}

func TestBlindSignatureIssuer(t *testing.T) {
	issuer, err := NewBlindSignatureIssuer(2048)
	if err != nil {
		t.Fatalf("Failed to create issuer: %v", err)
	}

	voterID := []byte("voter-12345")

	credential, err := issuer.IssueCredential(voterID)
	if err != nil {
		t.Fatalf("IssueCredential failed: %v", err)
	}

	if !issuer.VerifyCredential(voterID, credential) {
		t.Error("Credential verification failed")
	}

	wrongVoterID := []byte("voter-99999")
	if issuer.VerifyCredential(wrongVoterID, credential) {
		t.Error("Credential verification should fail for wrong voter ID")
	}
}

func TestBlindSignatureUniqueness(t *testing.T) {
	bs, err := NewBlindSignature(2048)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	message := []byte("test-message")

	sig1, _, _ := bs.BlindMessage(message)
	sig2, _, _ := bs.BlindMessage(message)

	if bytes.Equal(sig1.Bytes(), sig2.Bytes()) {
		t.Error("Blind messages should be different with different blinding factors")
	}
}

func BenchmarkBlindSignature(b *testing.B) {
	bs, _ := NewBlindSignature(2048)
	message := []byte("benchmark-message")
	blinded, blindFactor, _ := bs.BlindMessage(message)
	signature, _ := bs.SignBlind(blinded)

	b.Run("BlindMessage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bs.BlindMessage(message)
		}
	})

	b.Run("SignBlind", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bs.SignBlind(blinded)
		}
	})

	b.Run("UnblindMessage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bs.UnblindMessage(signature, blindFactor)
		}
	})

	b.Run("VerifyBlindSignature", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bs.VerifyBlindSignature(message, signature)
		}
	})
}
