package services

import (
	"fmt"
	"time"

	"github.com/blockchain-voting-system/pkg/fabricclient"
)

// Provisioner creates elections via the Fabric Gateway SDK so integration
// tests can run without manual `peer chaincode invoke` setup.
type Provisioner struct {
	Fabric fabricclient.Client
}

// NewProvisioner connects to the network using the same env vars the voting
// service uses, so tests share configuration.
func NewProvisioner(cfg fabricclient.Config) (*Provisioner, error) {
	fab, err := fabricclient.Connect(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect fabric: %w", err)
	}
	return &Provisioner{Fabric: fab}, nil
}

// ProvisionElection registers a new election and transitions it to VOTING.
// Returns the election ID on success.
func (p *Provisioner) ProvisionElection(electionID string) error {
	if electionID == "" {
		return fmt.Errorf("electionID is required")
	}

	// 1. Register. Candidates are fixed for the test.
	candidatesJSON := `["Alice","Bob"]`
	_, err := p.Fabric.Submit("RegisterElection",
		electionID,
		"Integration Test Election",
		candidatesJSON,
		"placeholder-homomorphic-pubkey", // signature only; tests don't decrypt
	)
	if err != nil {
		return fmt.Errorf("RegisterElection: %w", err)
	}

	// 2. Start voting.
	if _, err := p.Fabric.Submit("StartVoting", electionID); err != nil {
		return fmt.Errorf("StartVoting: %w", err)
	}

	return nil
}

// Close releases the underlying Fabric connection.
func (p *Provisioner) Close() error {
	if p.Fabric != nil {
		return p.Fabric.Close()
	}
	return nil
}

// FreshElectionID returns a unique election ID for a test run.
func FreshElectionID() string {
	return fmt.Sprintf("svc-test-%d", time.Now().UnixNano())
}
