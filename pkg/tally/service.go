package tally

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/keyceremony"
)

// Config controls a single tally run.
type Config struct {
	ElectionID       string
	CeremonyDir      string // directory with keyholder shares + publickey.json + ceremony.json
	PassphrasePrefix string // e.g. "dev-keyholder-"
	MaxSum           int64  // optional; defaults to ballotCount

	// Fabric client for chaincode interaction
	Fabric fabricclient.Client
}

// Service orchestrates tally runs.
type Service struct{}

// Run executes the full ceremony for an election:
//  1. Fetch encrypted tally from chaincode
//  2. Load ceremony metadata + public key
//  3. Load keyholder shares
//  4. Reconstruct private key
//  5. Decrypt per-candidate sums
//  6. Generate ZK proofs
//  7. Publish result to chaincode
func (s *Service) Run(cfg Config) (*Result, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	log.Printf("[tally] starting ceremony for election %q", cfg.ElectionID)

	// 1. Fetch the encrypted tally (per-candidate ciphertexts)
	tally, err := fetchEncryptedTally(cfg.Fabric, cfg.ElectionID)
	if err != nil {
		return nil, fmt.Errorf("fetch encrypted tally: %w", err)
	}
	log.Printf("[tally] fetched tally: %d ballots, %d candidates", tally.BallotCount, len(tally.Candidates))

	if tally.BallotCount == 0 {
		return nil, errors.New("election has no ballots")
	}

	// 2. Load ceremony metadata and public key
	meta, err := keyceremony.LoadCeremonyMetadata(cfg.CeremonyDir)
	if err != nil {
		return nil, fmt.Errorf("load ceremony metadata: %w", err)
	}
	publicKey, err := keyceremony.LoadPublicKey(filepath.Join(cfg.CeremonyDir, "publickey.json"))
	if err != nil {
		return nil, fmt.Errorf("load public key: %w", err)
	}

	// 3. Load keyholder shares
	sharePaths, err := findShareFiles(cfg.CeremonyDir, meta.Total)
	if err != nil {
		return nil, fmt.Errorf("find shares: %w", err)
	}
	if len(sharePaths) < meta.Threshold {
		return nil, fmt.Errorf("need %d shares, found %d", meta.Threshold, len(sharePaths))
	}
	sharePaths = sharePaths[:meta.Threshold] // use exactly threshold
	passphrases := make([]string, len(sharePaths))
	for i := range sharePaths {
		passphrases[i] = cfg.PassphrasePrefix + keyholderIDFromPath(sharePaths[i])
	}

	// 4. Reconstruct private key
	privateKey, err := reconstructKey(sharePaths, passphrases)
	if err != nil {
		return nil, fmt.Errorf("reconstruct key: %w", err)
	}
	privateKey.PublicKey = *publicKey
	log.Printf("[tally] reconstructed private key from %d shares", len(sharePaths))

	// 5. Decrypt per-candidate sums
	maxSum := cfg.MaxSum
	if maxSum == 0 {
		maxSum = tally.BallotCount
	}

	results := make([]CandidateResult, len(tally.Candidates))
	for i, candidate := range tally.Candidates {
		ct, err := parseCiphertext(tally.C1s[i], tally.C2s[i])
		if err != nil {
			return nil, fmt.Errorf("parse ciphertext for %q: %w", candidate, err)
		}

		sum, err := privateKey.DecryptVote(ct, maxSum)
		if err != nil {
			return nil, fmt.Errorf("decrypt %q: %w", candidate, err)
		}

		// 6. Generate ZK proof that decryption is correct
		m := new(big.Int).Exp(publicKey.G, big.NewInt(sum), publicKey.P)
		proof, err := zkproofs.GenerateDecryptionProof(
			ct.C1, ct.C2, m,
			publicKey.Y, publicKey.P, publicKey.G,
			privateKey.X,
		)
		if err != nil {
			return nil, fmt.Errorf("generate proof for %q: %w", candidate, err)
		}

		results[i] = CandidateResult{
			Candidate: candidate,
			Sum:       sum,
			Proof:     proof,
		}
		log.Printf("[tally] candidate %q: sum=%d, proof generated", candidate, sum)
	}

	// 7. Publish result
	result := &Result{
		ElectionID:       cfg.ElectionID,
		CandidateResults: results,
		TotalBallots:     tally.BallotCount,
	}
	if err := publishResult(cfg.Fabric, result); err != nil {
		return nil, fmt.Errorf("publish result: %w", err)
	}
	log.Printf("[tally] ceremony complete: %d candidates, %d ballots", len(results), tally.BallotCount)

	return result, nil
}

// --- helpers ---

func validateConfig(cfg Config) error {
	if cfg.ElectionID == "" {
		return errors.New("electionID is required")
	}
	if cfg.CeremonyDir == "" {
		return errors.New("ceremonyDir is required")
	}
	if cfg.PassphrasePrefix == "" {
		return errors.New("passphrasePrefix is required")
	}
	if cfg.Fabric == nil {
		return errors.New("fabric client is required")
	}
	return nil
}

// encryptedTally mirrors the chaincode's PerCandidateTally.
type encryptedTally struct {
	Candidates  []string `json:"candidates"`
	C1s         []string `json:"c1s"`
	C2s         []string `json:"c2s"`
	BallotCount int64    `json:"ballotCount"`
}

func fetchEncryptedTally(fab fabricclient.Client, electionID string) (*encryptedTally, error) {
	raw, err := fab.Evaluate("GetEncryptedTally", electionID)
	if err != nil {
		return nil, err
	}
	var t encryptedTally
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("parse tally: %w", err)
	}
	if len(t.Candidates) != len(t.C1s) || len(t.Candidates) != len(t.C2s) {
		return nil, fmt.Errorf("tally shape mismatch: %d candidates, %d C1s, %d C2s",
			len(t.Candidates), len(t.C1s), len(t.C2s))
	}
	return &t, nil
}

func reconstructKey(sharePaths, passphrases []string) (*elgamal.PrivateKey, error) {
	// Load first share to get outputLen
	_, outLen, err := keyceremony.LoadShare(sharePaths[0], passphrases[0])
	if err != nil {
		return nil, fmt.Errorf("load share[0]: %w", err)
	}
	return keyceremony.ReconstructPrivateKey(sharePaths, passphrases, outLen)
}

func parseCiphertext(c1s, c2s string) (*elgamal.Ciphertext, error) {
	c1, ok := new(big.Int).SetString(c1s, 10)
	if !ok {
		return nil, errors.New("invalid C1")
	}
	c2, ok := new(big.Int).SetString(c2s, 10)
	if !ok {
		return nil, errors.New("invalid C2")
	}
	return &elgamal.Ciphertext{C1: c1, C2: c2}, nil
}

func publishResult(fab fabricclient.Client, r *Result) error {
	candidateResultsJSON, err := serializeCandidateResults(r.CandidateResults)
	if err != nil {
		return err
	}
	_, err = fab.Submit("PublishTally", r.ElectionID, candidateResultsJSON, fmt.Sprintf("%d", r.TotalBallots))
	return err
}

// findShareFiles returns sorted keyholder-N.share paths in the ceremony dir.
func findShareFiles(dir string, total int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var shares []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "keyholder-") && strings.HasSuffix(e.Name(), ".share") {
			shares = append(shares, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(shares)
	return shares, nil
}

// keyholderIDFromPath extracts "1" from "keyholder-1.share".
func keyholderIDFromPath(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".share")
	base = strings.TrimPrefix(base, "keyholder-")
	return base
}

// GetResult fetches the published tally from the chaincode.
func (s *Service) GetResult(fab fabricclient.Client, electionID string) (*Result, error) {
	raw, err := fab.Evaluate("GetTally", electionID)
	if err != nil {
		return nil, err
	}
	var rawTally struct {
		ElectionID       string `json:"electionId"`
		CandidateResults []struct {
			Candidate string `json:"candidate"`
			Sum       int64  `json:"sum"`
			Proof     string `json:"proof"`
		} `json:"candidateResults"`
		TotalBallots int64 `json:"totalBallots"`
	}
	if err := json.Unmarshal(raw, &rawTally); err != nil {
		return nil, fmt.Errorf("parse tally result: %w", err)
	}

	results := make([]CandidateResult, len(rawTally.CandidateResults))
	for i, cr := range rawTally.CandidateResults {
		var proof zkproofs.DecryptionProof
		if err := json.Unmarshal([]byte(cr.Proof), &proof); err != nil {
			return nil, fmt.Errorf("parse proof for %q: %w", cr.Candidate, err)
		}
		results[i] = CandidateResult{
			Candidate: cr.Candidate,
			Sum:       cr.Sum,
			Proof:     &proof,
		}
	}

	return &Result{
		ElectionID:       rawTally.ElectionID,
		CandidateResults: results,
		TotalBallots:     rawTally.TotalBallots,
	}, nil
}

// sanity: unused import guard
var _ = time.Now
