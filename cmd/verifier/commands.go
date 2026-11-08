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
	// Don't defer close — server runs forever
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
		// /api/ballot/{election}/{ballot}
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
