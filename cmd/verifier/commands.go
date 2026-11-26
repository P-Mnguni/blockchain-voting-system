package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/verifier"
)

// fabricFlags holds the common Fabric connection flags.
type fabricFlags struct {
	mspID     string
	endpoint  string
	hostname  string
	tlsCA     string
	cert      string
	key       string
	channel   string
	chaincode string
}

func (f *fabricFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.mspID, "msp-id", "", "MSP ID")
	fs.StringVar(&f.endpoint, "endpoint", "", "peer endpoint (host:port)")
	fs.StringVar(&f.hostname, "hostname", "", "TLS server name")
	fs.StringVar(&f.tlsCA, "tls-ca", "", "TLS CA certificate path")
	fs.StringVar(&f.cert, "cert", "", "user certificate path")
	fs.StringVar(&f.key, "key", "", "user private key path")
	fs.StringVar(&f.channel, "channel", "electionchannel", "channel name")
	fs.StringVar(&f.chaincode, "chaincode", "ballot", "chaincode name")
}

func (f *fabricFlags) connect() (fabricclient.Client, error) {
	required := []struct{ name, val string }{
		{"msp-id", f.mspID},
		{"endpoint", f.endpoint},
		{"tls-ca", f.tlsCA},
		{"cert", f.cert},
		{"key", f.key},
	}
	for _, r := range required {
		if r.val == "" {
			return nil, fmt.Errorf("--%s is required", r.name)
		}
	}
	hostname := f.hostname
	if hostname == "" {
		hostname = "peer0.electoralcommission.example.com"
	}
	return fabricclient.Connect(fabricclient.Config{
		MSPID:         f.mspID,
		PeerEndpoint:  f.endpoint,
		PeerHostname:  hostname,
		TLSCACertPath: f.tlsCA,
		CertPath:      f.cert,
		KeyPath:       f.key,
		ChannelName:   f.channel,
		ChaincodeName: f.chaincode,
	})
}

func runTally(args []string) error {
	fs := flag.NewFlagSet("tally", flag.ExitOnError)
	electionID := fs.String("election", "", "election ID")
	verbose := fs.Bool("verbose", false, "show step-by-step verification detail")
	var ff fabricFlags
	ff.register(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *electionID == "" {
		return fmt.Errorf("--election is required")
	}

	fab, err := ff.connect()
	if err != nil {
		return err
	}
	defer fab.Close()

	v := &verifier.Verifier{Fabric: fab}

	if *verbose {
		return runVerboseTally(v, *electionID)
	}

	res, err := v.VerifyTally(*electionID)
	if err != nil {
		return err
	}
	printTallyResult(res)
	if !res.ProofsVerified {
		os.Exit(2)
	}
	return nil
}

// --- verbose mode ---

func runVerboseTally(v *verifier.Verifier, electionID string) error {
	start := time.Now()

	fmt.Printf("Verifying election %s\n", electionID)
	fmt.Printf("Stateless, read-only, no secret key.\n\n")

	// Step 1: fetch election
	stepStart := time.Now()
	fmt.Printf("[1/5] Fetching election from ledger...\n")
	election, err := v.FetchElection(electionID)
	if err != nil {
		fmt.Printf("      FAILED: %v\n", err)
		fmt.Println("\n=== VERIFICATION FAILED ===")
		os.Exit(2)
	}
	fmt.Printf("      OK  phase=%s  candidates=%d\n", election.Phase, len(election.Candidates))
	fmt.Printf("      elapsed: %s\n\n", time.Since(stepStart).Round(time.Millisecond))

	// Step 2: fetch published tally
	stepStart = time.Now()
	fmt.Printf("[2/5] Fetching published tally from chain...\n")
	tally, err := v.FetchPublishedTally(electionID)
	if err != nil {
		fmt.Printf("      FAILED: %v\n", err)
		fmt.Println("\n=== VERIFICATION FAILED ===")
		os.Exit(2)
	}
	fmt.Printf("      OK  ballots=%d\n", tally.TotalBallots)
	for _, r := range tally.CandidateResults {
		fmt.Printf("      published: %s = %d\n", r.Candidate, r.Sum)
	}
	fmt.Printf("      elapsed: %s\n\n", time.Since(stepStart).Round(time.Millisecond))

	// Step 3: fetch encrypted tally
	stepStart = time.Now()
	fmt.Printf("[3/5] Fetching encrypted tally ciphertext from chain...\n")
	encTally, err := v.FetchEncryptedTally(electionID)
	if err != nil {
		fmt.Printf("      FAILED: %v\n", err)
		fmt.Println("\n=== VERIFICATION FAILED ===")
		os.Exit(2)
	}
	for i, c := range encTally.Candidates {
		fmt.Printf("      %s: C1=%s...  C2=%s...\n",
			c, truncate(encTally.C1s[i], 16), truncate(encTally.C2s[i], 16))
	}
	fmt.Printf("      elapsed: %s\n\n", time.Since(stepStart).Round(time.Millisecond))

	// Step 4: verify ZK proofs
	stepStart = time.Now()
	fmt.Printf("[4/5] Verifying ZK proofs (Chaum-Pedersen)...\n")
	proofResults, err := v.VerifyProofs(electionID)
	if err != nil {
		fmt.Printf("      FAILED: %v\n", err)
		fmt.Println("\n=== VERIFICATION FAILED ===")
		os.Exit(2)
	}
	allValid := true
	for _, pr := range proofResults {
		if pr.Valid {
			fmt.Printf("      OK  %s: sum=%d  proof=VALID  (%s)\n",
				pr.Candidate, pr.Sum, pr.Duration.Round(time.Microsecond))
		} else {
			fmt.Printf("      FAIL  %s: sum=%d  proof=INVALID  %s\n",
				pr.Candidate, pr.Sum, pr.Error)
			allValid = false
		}
	}
	fmt.Printf("      elapsed: %s\n\n", time.Since(stepStart).Round(time.Millisecond))

	// Step 5: recompute homomorphic sum
	stepStart = time.Now()
	fmt.Printf("[5/5] Recomputing homomorphic sum from raw ballots...\n")
	recompute, err := v.RecomputeTally(electionID)
	if err != nil {
		fmt.Printf("      FAILED: %v\n", err)
		fmt.Println("\n=== VERIFICATION FAILED ===")
		os.Exit(2)
	}
	match := recompute.Matches
	if match {
		fmt.Printf("      OK  computed ciphertext matches on-chain tally\n")
	} else {
		fmt.Printf("      FAIL  computed ciphertext does NOT match on-chain tally\n")
		for _, m := range recompute.Mismatches {
			fmt.Printf("            %s\n", m)
		}
		allValid = false
	}
	fmt.Printf("      elapsed: %s\n\n", time.Since(stepStart).Round(time.Millisecond))

	// Final
	fmt.Printf("Total elapsed: %s\n\n", time.Since(start).Round(time.Millisecond))
	if allValid && match {
		fmt.Println("=== VERIFIED ===")
		fmt.Println("No secret key. No trusted party. Just public ledger data + ZK proofs.")
		return nil
	}
	fmt.Println("=== VERIFICATION FAILED ===")
	os.Exit(2)
	return nil
}

// truncate shows the first n chars of s, with an ellipsis if longer.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- non-verbose output (existing) ---

func printTallyResult(res *verifier.Result) {
	fmt.Printf("\n=== Election %s ===\n", res.ElectionID)
	fmt.Printf("Phase: %s\n", res.Phase)
	fmt.Printf("Ballots cast: %d\n\n", res.BallotsCast)

	fmt.Println("Per-candidate results:")
	for _, r := range res.CandidateResults {
		mark := "✓"
		if !r.ProofValid {
			mark = "✗"
		}
		fmt.Printf("  %s %-12s sum=%d\n", mark, r.Candidate, r.Sum)
		if r.VerificationErr != "" {
			fmt.Printf("      ERROR: %s\n", r.VerificationErr)
		}
	}

	fmt.Printf("\nProofs verified: %v\n", boolMark(res.ProofsVerified))
	fmt.Printf("Ciphertext match: %v\n", boolMark(res.CiphertextMatch))

	if len(res.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range res.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}
	if res.ProofsVerified && res.CiphertextMatch {
		fmt.Println("\n=== VERIFIED ===")
	} else {
		fmt.Println("\n=== VERIFICATION FAILED ===")
	}
}

func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func runBallot(args []string) error {
	fs := flag.NewFlagSet("ballot", flag.ExitOnError)
	electionID := fs.String("election", "", "election ID")
	ballotID := fs.String("ballot-id", "", "ballot ID")
	var ff fabricFlags
	ff.register(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *electionID == "" || *ballotID == "" {
		return fmt.Errorf("--election and --ballot-id are required")
	}

	fab, err := ff.connect()
	if err != nil {
		return err
	}
	defer fab.Close()

	v := &verifier.Verifier{Fabric: fab}
	bv, err := v.VerifyBallot(*electionID, *ballotID)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== Ballot %s ===\n", bv.BallotID)
	fmt.Printf("Election: %s\n", bv.ElectionID)
	fmt.Printf("Found on ledger: %v\n", boolMark(bv.Found))
	fmt.Printf("Cast at: %s\n", time.Unix(bv.CastAt, 0).UTC().Format(time.RFC3339))
	fmt.Printf("Encrypted votes: %d\n", len(bv.EncryptedVotes))
	if bv.Found {
		fmt.Println("\n=== VERIFIED: ballot is on the ledger ===")
	} else {
		fmt.Println("\n=== NOT FOUND ===")
		os.Exit(2)
	}
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "8090", "HTTP port")
	var ff fabricFlags
	ff.register(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}

	fab, err := ff.connect()
	if err != nil {
		return err
	}
	v := &verifier.Verifier{Fabric: fab}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/verify/", func(w http.ResponseWriter, r *http.Request) {
		electionID := r.URL.Path[len("/api/verify/"):]
		if electionID == "" {
			http.Error(w, "missing election ID", 400)
			return
		}
		res, err := v.VerifyTally(electionID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
	mux.HandleFunc("/api/ballot/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitPath(r.URL.Path[len("/api/ballot/"):])
		if len(parts) != 2 {
			http.Error(w, "expected /api/ballot/{election}/{ballot}", 400)
			return
		}
		bv, err := v.VerifyBallot(parts[0], parts[1])
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bv)
	})
	mux.HandleFunc("/", serveIndex)

	log.Printf("listening on :%s", *port)
	return http.ListenAndServe(":"+*port, mux)
}

func splitPath(s string) []string {
	var out []string
	current := ""
	for _, c := range s {
		if c == '/' {
			if current != "" {
				out = append(out, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(indexHTML))
}
