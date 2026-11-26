// Package verifier independently verifies the results of an election.
//
// It is stateless and read-only: it only queries public ledger data, never
// touches the private decryption key, and relies on the published ZK proofs
// to confirm that decryption was done correctly.
package verifier

import (
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/zkproofs"
	"github.com/blockchain-voting-system/pkg/fabricclient"
)

// Client is the subset of fabricclient.Client the verifier needs.
type Client interface {
	Evaluate(fn string, args ...string) ([]byte, error)
}

// Result holds the verification outcome for one election.
type Result struct {
	ElectionID       string            `json:"electionId"`
	Phase            string            `json:"phase"`
	BallotsCast      int64             `json:"ballotsCast"`
	CandidateResults []CandidateResult `json:"candidateResults"`
	ProofsVerified   bool              `json:"proofsVerified"`
	CiphertextMatch  bool              `json:"ciphertextMatch"`
	Errors           []string          `json:"errors"`
}

// CandidateResult is a verified candidate result.
type CandidateResult struct {
	Candidate       string `json:"candidate"`
	Sum             int64  `json:"sum"`
	ProofValid      bool   `json:"proofValid"`
	VerificationErr string `json:"verificationErr,omitempty"`
}

// ElectionInfo is a minimal view of an election for verbose output.
type ElectionInfo struct {
	ElectionID string   `json:"electionId"`
	Phase      string   `json:"phase"`
	Candidates []string `json:"candidates"`
}

// Verifier performs election verification against a Fabric client.
type Verifier struct {
	Fabric Client
}

/*
VerifyTally performs the full verification:
 1. Fetch the published tally result from the chaincode
 2. Fetch the encrypted tally (the source of truth for sums)
 3. For each candidate:
    a. Verify the ZK proof: ciphertext decrypts to g^sum
    b. (Optional) Recompute from raw ballots and check it matches the
    on-chain encrypted tally
*/
func (v *Verifier) VerifyTally(electionID string) (*Result, error) {
	if v.Fabric == nil {
		return nil, fmt.Errorf("fabric client is required")
	}
	if electionID == "" {
		return nil, fmt.Errorf("electionID is required")
	}

	res := &Result{
		ElectionID: electionID,
		Errors:     []string{},
	}

	// 1. Fetch election phase
	electionRaw, err := v.Fabric.Evaluate("GetElection", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch election: %w", err)
	}
	var election struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(electionRaw, &election); err != nil {
		return nil, fmt.Errorf("parse election: %w", err)
	}
	res.Phase = election.Phase

	// 2. Fetch published tally
	tallyRaw, err := v.Fabric.Evaluate("GetTally", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch tally: %w", err)
	}
	var chainTally struct {
		ElectionID       string `json:"electionId"`
		CandidateResults []struct {
			Candidate string `json:"candidate"`
			Sum       int64  `json:"sum"`
			Proof     string `json:"proof"`
		} `json:"candidateResults"`
		TotalBallots int64 `json:"totalBallots"`
	}
	if err := json.Unmarshal(tallyRaw, &chainTally); err != nil {
		return nil, fmt.Errorf("parse tally: %w", err)
	}
	res.BallotsCast = chainTally.TotalBallots

	// 3. Fetch encrypted tally (the source ciphertexts)
	encRaw, err := v.Fabric.Evaluate("GetEncryptedTally", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch encrypted tally: %w", err)
	}
	var encTally struct {
		Candidates []string `json:"candidates"`
		C1s        []string `json:"c1s"`
		C2s        []string `json:"c2s"`
	}
	if err := json.Unmarshal(encRaw, &encTally); err != nil {
		return nil, fmt.Errorf("parse encrypted tally: %w", err)
	}

	// 4. Fetch the election's public key (needed for proof verification)
	pubKey, err := v.getPublicKey(electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch public key: %w", err)
	}

	// 5. Verify each candidate's proof against the encrypted ciphertext
	res.ProofsVerified = true
	for i, cr := range chainTally.CandidateResults {
		verifResult := CandidateResult{
			Candidate: cr.Candidate,
			Sum:       cr.Sum,
		}

		// The proof must verify against the ciphertext at position i
		if i >= len(encTally.C1s) || i >= len(encTally.C2s) {
			verifResult.VerificationErr = "missing ciphertext"
			verifResult.ProofValid = false
			res.ProofsVerified = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: missing ciphertext", cr.Candidate))
			res.CandidateResults = append(res.CandidateResults, verifResult)
			continue
		}

		c1, ok1 := new(big.Int).SetString(encTally.C1s[i], 10)
		c2, ok2 := new(big.Int).SetString(encTally.C2s[i], 10)
		if !ok1 || !ok2 {
			verifResult.VerificationErr = "invalid ciphertext encoding"
			verifResult.ProofValid = false
			res.ProofsVerified = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: invalid ciphertext", cr.Candidate))
			res.CandidateResults = append(res.CandidateResults, verifResult)
			continue
		}

		var proof zkproofs.DecryptionProof
		if err := json.Unmarshal([]byte(cr.Proof), &proof); err != nil {
			verifResult.VerificationErr = fmt.Sprintf("parse proof: %v", err)
			verifResult.ProofValid = false
			res.ProofsVerified = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", cr.Candidate, verifResult.VerificationErr))
			res.CandidateResults = append(res.CandidateResults, verifResult)
			continue
		}

		// Compute g^sum mod p
		m := new(big.Int).Exp(pubKey.G, big.NewInt(cr.Sum), pubKey.P)

		// Verify the Chaum-Pedersen proof
		valid := zkproofs.VerifyDecryptionProof(c1, c2, m, pubKey.Y, pubKey.P, pubKey.G, &proof)
		verifResult.ProofValid = valid
		if !valid {
			res.ProofsVerified = false
			verifResult.VerificationErr = "proof does not verify"
			res.Errors = append(res.Errors, fmt.Sprintf("%s: proof does not verify", cr.Candidate))
		}
		res.CandidateResults = append(res.CandidateResults, verifResult)
	}

	// 6. Recompute the encrypted tally from raw ballots and compare
	//    This is a "belt and suspenders" check: it proves the on-chain
	//    encrypted tally is the real sum of the on-chain ballots.
	recomputedMatch, recomputeErr := v.recomputeAndCompare(electionID, encTally.C1s, encTally.C2s)
	res.CiphertextMatch = recomputedMatch
	if recomputeErr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("recompute: %v", recomputeErr))
	} else if !recomputedMatch {
		res.Errors = append(res.Errors, "recomputed ciphertext does not match on-chain tally")
	}

	return res, nil
}

/*
VerifyBallot confirms that a specific ballot exists on the ledger and
returns its encrypted votes.
*/
func (v *Verifier) VerifyBallot(electionID, ballotID string) (*BallotVerification, error) {
	if v.Fabric == nil {
		return nil, fmt.Errorf("fabric client is required")
	}
	if electionID == "" || ballotID == "" {
		return nil, fmt.Errorf("electionID and ballotID are required")
	}

	raw, err := v.Fabric.Evaluate("GetBallot", electionID, ballotID)
	if err != nil {
		return nil, fmt.Errorf("fetch ballot: %w", err)
	}

	var b struct {
		BallotID       string   `json:"ballotId"`
		ElectionID     string   `json:"electionId"`
		EncryptedVotes []string `json:"encryptedVotes"`
		CastAt         int64    `json:"castAt"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("parse ballot: %w", err)
	}

	return &BallotVerification{
		BallotID:       b.BallotID,
		ElectionID:     b.ElectionID,
		EncryptedVotes: b.EncryptedVotes,
		CastAt:         b.CastAt,
		Found:          true,
	}, nil
}

// BallotVerification is the result of a single ballot lookup.
type BallotVerification struct {
	BallotID       string   `json:"ballotId"`
	ElectionID     string   `json:"electionId"`
	EncryptedVotes []string `json:"encryptedVotes"`
	CastAt         int64    `json:"castAt"`
	Found          bool     `json:"found"`
}

// FetchElection fetches election metadata.
func (v *Verifier) FetchElection(electionID string) (*ElectionInfo, error) {
	raw, err := v.Fabric.Evaluate("GetElection", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch election: %w", err)
	}
	var e ElectionInfo
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("parse election: %w", err)
	}
	return &e, nil
}

// PublishedTally is a minimal view of the published tally.
type PublishedTally struct {
	ElectionID       string
	TotalBallots     int64
	CandidateResults []PublishedCandidate
}

type PublishedCandidate struct {
	Candidate string
	Sum       int64
	ProofJSON string
}

// FetchPublishedTally fetches the published tally from the chain.
func (v *Verifier) FetchPublishedTally(electionID string) (*PublishedTally, error) {
	raw, err := v.Fabric.Evaluate("GetTally", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch tally: %w", err)
	}
	var t struct {
		ElectionID       string `json:"electionId"`
		CandidateResults []struct {
			Candidate string `json:"candidate"`
			Sum       int64  `json:"sum"`
			Proof     string `json:"proof"`
		} `json:"candidateResults"`
		TotalBallots int64 `json:"totalBallots"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("parse tally: %w", err)
	}
	out := &PublishedTally{
		ElectionID:   t.ElectionID,
		TotalBallots: t.TotalBallots,
	}
	for _, c := range t.CandidateResults {
		out.CandidateResults = append(out.CandidateResults, PublishedCandidate{
			Candidate: c.Candidate,
			Sum:       c.Sum,
			ProofJSON: c.Proof,
		})
	}
	return out, nil
}

// EncryptedTallyView is a minimal view of the encrypted tally.
type EncryptedTallyView struct {
	Candidates []string
	C1s        []string
	C2s        []string
}

// FetchEncryptedTally fetches the on-chain ciphertext.
func (v *Verifier) FetchEncryptedTally(electionID string) (*EncryptedTallyView, error) {
	raw, err := v.Fabric.Evaluate("GetEncryptedTally", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch encrypted tally: %w", err)
	}
	var e EncryptedTallyView
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("parse encrypted tally: %w", err)
	}
	return &e, nil
}

// ProofVerification is the result of verifying one candidate's proof.
type ProofVerification struct {
	Candidate string
	Sum       int64
	Valid     bool
	Duration  time.Duration
	Error     string
}

// VerifyProofs verifies each candidate's ZK proof.
func (v *Verifier) VerifyProofs(electionID string) ([]ProofVerification, error) {
	tally, err := v.FetchPublishedTally(electionID)
	if err != nil {
		return nil, err
	}
	enc, err := v.FetchEncryptedTally(electionID)
	if err != nil {
		return nil, err
	}
	pk, err := v.getPublicKey(electionID)
	if err != nil {
		return nil, err
	}

	var results []ProofVerification
	for i, cr := range tally.CandidateResults {
		pv := ProofVerification{Candidate: cr.Candidate, Sum: cr.Sum}
		start := time.Now()

		if i >= len(enc.C1s) || i >= len(enc.C2s) {
			pv.Valid = false
			pv.Error = "missing ciphertext"
			pv.Duration = time.Since(start)
			results = append(results, pv)
			continue
		}

		c1, ok1 := new(big.Int).SetString(enc.C1s[i], 10)
		c2, ok2 := new(big.Int).SetString(enc.C2s[i], 10)
		if !ok1 || !ok2 {
			pv.Valid = false
			pv.Error = "invalid ciphertext encoding"
			pv.Duration = time.Since(start)
			results = append(results, pv)
			continue
		}

		var proof zkproofs.DecryptionProof
		if err := json.Unmarshal([]byte(cr.ProofJSON), &proof); err != nil {
			pv.Valid = false
			pv.Error = fmt.Sprintf("parse proof: %v", err)
			pv.Duration = time.Since(start)
			results = append(results, pv)
			continue
		}

		m := new(big.Int).Exp(pk.G, big.NewInt(cr.Sum), pk.P)
		pv.Valid = zkproofs.VerifyDecryptionProof(c1, c2, m, pk.Y, pk.P, pk.G, &proof)
		pv.Duration = time.Since(start)
		if !pv.Valid {
			pv.Error = "proof does not verify"
		}
		results = append(results, pv)
	}
	return results, nil
}

// RecomputeResult is the result of recomputing the homomorphic sum.
type RecomputeResult struct {
	Matches    bool
	Mismatches []string
}

// RecomputeTally fetches all ballots, sums their ciphertexts, and compares
// against the on-chain encrypted tally.
func (v *Verifier) RecomputeTally(electionID string) (*RecomputeResult, error) {
	enc, err := v.FetchEncryptedTally(electionID)
	if err != nil {
		return nil, err
	}
	return v.recomputeAndCompareRecheck(electionID, enc.C1s, enc.C2s)
}

// recomputeAndCompareRecheck returns detailed mismatch info.
func (v *Verifier) recomputeAndCompareRecheck(electionID string, expectedC1s, expectedC2s []string) (*RecomputeResult, error) {
	raw, err := v.Fabric.Evaluate("GetAllBallots", electionID)
	if err != nil {
		return nil, fmt.Errorf("fetch ballots: %w", err)
	}
	var ballots []struct {
		EncryptedVotes []string `json:"encryptedVotes"`
	}
	if err := json.Unmarshal(raw, &ballots); err != nil {
		return nil, fmt.Errorf("parse ballots: %w", err)
	}
	if len(ballots) == 0 {
		return &RecomputeResult{Matches: false, Mismatches: []string{"no ballots"}}, nil
	}

	num := len(expectedC1s)
	sumC1 := make([]*big.Int, num)
	sumC2 := make([]*big.Int, num)
	for i := 0; i < num; i++ {
		sumC1[i] = big.NewInt(1)
		sumC2[i] = big.NewInt(1)
	}
	for _, b := range ballots {
		if len(b.EncryptedVotes) != num {
			return nil, fmt.Errorf("ballot has %d votes, expected %d", len(b.EncryptedVotes), num)
		}
		for i, vs := range b.EncryptedVotes {
			var ct struct{ C1, C2 string }
			if err := json.Unmarshal([]byte(vs), &ct); err != nil {
				return nil, fmt.Errorf("parse ciphertext: %w", err)
			}
			c1, _ := new(big.Int).SetString(ct.C1, 10)
			c2, _ := new(big.Int).SetString(ct.C2, 10)
			if c1 != nil {
				sumC1[i].Mul(sumC1[i], c1)
			}
			if c2 != nil {
				sumC2[i].Mul(sumC2[i], c2)
			}
		}
	}

	res := &RecomputeResult{Matches: true}
	for i := 0; i < num; i++ {
		wantC1, _ := new(big.Int).SetString(expectedC1s[i], 10)
		wantC2, _ := new(big.Int).SetString(expectedC2s[i], 10)
		if wantC1 != nil && sumC1[i].Cmp(wantC1) != 0 {
			res.Matches = false
			res.Mismatches = append(res.Mismatches, fmt.Sprintf("candidate %d: C1 mismatch", i))
		}
		if wantC2 != nil && sumC2[i].Cmp(wantC2) != 0 {
			res.Matches = false
			res.Mismatches = append(res.Mismatches, fmt.Sprintf("candidate %d: C2 mismatch", i))
		}
	}
	return res, nil
}

// --- internal helpers ---

type publicKey struct {
	G, P, Y *big.Int
}

func (v *Verifier) getPublicKey(electionID string) (*publicKey, error) {
	raw, err := v.Fabric.Evaluate("GetElection", electionID)
	if err != nil {
		return nil, err
	}
	var election struct {
		HomomorphicPubKey string `json:"homomorphicPubKey"`
	}
	if err := json.Unmarshal(raw, &election); err != nil {
		return nil, err
	}
	if election.HomomorphicPubKey == "" {
		return nil, fmt.Errorf("election has no homomorphic public key")
	}

	var rawPK struct {
		G, P, Y string
	}
	if err := json.Unmarshal([]byte(election.HomomorphicPubKey), &rawPK); err != nil {
		return nil, fmt.Errorf("parse public key JSON: %w", err)
	}

	g, ok := new(big.Int).SetString(rawPK.G, 10)
	if !ok {
		return nil, fmt.Errorf("invalid G")
	}
	p, ok := new(big.Int).SetString(rawPK.P, 10)
	if !ok {
		return nil, fmt.Errorf("invalid P")
	}
	y, ok := new(big.Int).SetString(rawPK.Y, 10)
	if !ok {
		return nil, fmt.Errorf("invalid Y")
	}
	return &publicKey{G: g, P: p, Y: y}, nil
}

/*
recomputeAndCompare fetches all ballots, sums their ciphertexts per candidate,
and compares against the expected C1s/C2s.
*/
func (v *Verifier) recomputeAndCompare(electionID string, expectedC1s, expectedC2s []string) (bool, error) {
	raw, err := v.Fabric.Evaluate("GetAllBallots", electionID)
	if err != nil {
		return false, fmt.Errorf("fetch ballots: %w", err)
	}

	var ballots []struct {
		EncryptedVotes []string `json:"encryptedVotes"`
	}
	if err := json.Unmarshal(raw, &ballots); err != nil {
		return false, fmt.Errorf("parse ballots: %w", err)
	}
	if len(ballots) == 0 {
		return false, fmt.Errorf("no ballots to recompute")
	}

	numCandidates := len(expectedC1s)
	sumC1 := make([]*big.Int, numCandidates)
	sumC2 := make([]*big.Int, numCandidates)
	for i := 0; i < numCandidates; i++ {
		sumC1[i] = big.NewInt(1)
		sumC2[i] = big.NewInt(1)
	}

	for _, b := range ballots {
		if len(b.EncryptedVotes) != numCandidates {
			return false, fmt.Errorf("ballot has %d votes, expected %d", len(b.EncryptedVotes), numCandidates)
		}
		for i, v := range b.EncryptedVotes {
			var ct struct{ C1, C2 string }
			if err := json.Unmarshal([]byte(v), &ct); err != nil {
				return false, fmt.Errorf("parse ciphertext: %w", err)
			}
			c1, ok := new(big.Int).SetString(ct.C1, 10)
			if !ok {
				return false, fmt.Errorf("invalid C1")
			}
			c2, ok := new(big.Int).SetString(ct.C2, 10)
			if !ok {
				return false, fmt.Errorf("invalid C2")
			}
			sumC1[i].Mul(sumC1[i], c1)
			sumC2[i].Mul(sumC2[i], c2)
		}
	}

	// Compare against expected
	for i := 0; i < numCandidates; i++ {
		wantC1, _ := new(big.Int).SetString(expectedC1s[i], 10)
		wantC2, _ := new(big.Int).SetString(expectedC2s[i], 10)
		if sumC1[i].Cmp(wantC1) != 0 {
			return false, fmt.Errorf("candidate %d: C1 mismatch", i)
		}
		if sumC2[i].Cmp(wantC2) != 0 {
			return false, fmt.Errorf("candidate %d: C2 mismatch", i)
		}
	}
	return true, nil
}

// Ensure fabricclient is referenced (documents the dependency)
var _ fabricclient.Client = nil
