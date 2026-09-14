package crypto

import (
	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/crypto/shamir"
	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
)

type (
	Share = shamir.Share

	BlindSignature       = blind.BlindSignature
	BlindSignatureIssuer = blind.BlindSignatureIssuer

	PublicKey  = elgamal.PublicKey
	PrivateKey = elgamal.PrivateKey
	Ciphertext = elgamal.Ciphertext

	DecryptionProof = zkproofs.DecryptionProof
	TallyProof      = zkproofs.TallyProof
)

var (
	SplitShamir   = shamir.Split
	CombineShamir = shamir.Combine

	NewElGamal       = elgamal.NewElGamal
	ElGamalEncrypt   = (*elgamal.PublicKey).Encrypt
	ElGamalDecrypt   = (*elgamal.PrivateKey).Decrypt
	ElGamalAdd       = elgamal.Add
	ElGamalAddMany   = elgamal.AddMany
	ElGamalScalarMul = elgamal.ScalarMul

	NewBlindSignature = blind.NewBlindSignature
	NewBlindIssuer    = blind.NewBlindSignatureIssuer

	GenerateDecryptionProof = zkproofs.GenerateDecryptionProof
	VerifyDecryptionProof   = zkproofs.VerifyDecryptionProof
	GenerateTallyProof      = zkproofs.GenerateTallyProof
	VerifyTallyProof        = zkproofs.VerifyTallyProof
)
