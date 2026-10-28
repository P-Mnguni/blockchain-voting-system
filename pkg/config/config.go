// Package config loads service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"time"
)

// Voting holds the full configuration for the voting-service binary.
type Voting struct {
	IdentityPort  string
	CastingPort   string
	ElectionID    string
	CredentialTTL time.Duration
	Fabric        Fabric
}

// Fabric holds the Gateway SDK configuration.
type Fabric struct {
	MSPID         string
	PeerEndpoint  string
	PeerHostname  string
	TLSCACertPath string
	CertPath      string
	KeyPath       string
	ChannelName   string
	ChaincodeName string
}

// LoadVoting reads the config from env vars.
func LoadVoting() (Voting, error) {
	electionID := os.Getenv("ELECTION_ID")
	if electionID == "" {
		return Voting{}, fmt.Errorf("ELECTION_ID is required")
	}

	ttl := 24 * time.Hour
	if v := os.Getenv("CREDENTIAL_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return Voting{}, fmt.Errorf("CREDENTIAL_TTL: %w", err)
		}
		ttl = parsed
	}

	required := []string{
		"FABRIC_MSP_ID",
		"FABRIC_ENDPOINT",
		"FABRIC_TLS_CA",
		"FABRIC_CERT",
		"FABRIC_KEY",
	}
	for _, k := range required {
		if os.Getenv(k) == "" {
			return Voting{}, fmt.Errorf("%s is required", k)
		}
	}

	return Voting{
		IdentityPort:  envOrDefault("IDENTITY_PORT", "8080"),
		CastingPort:   envOrDefault("CASTING_PORT", "8081"),
		ElectionID:    electionID,
		CredentialTTL: ttl,
		Fabric: Fabric{
			MSPID:         os.Getenv("FABRIC_MSP_ID"),
			PeerEndpoint:  os.Getenv("FABRIC_ENDPOINT"),
			PeerHostname:  envOrDefault("FABRIC_HOSTNAME", "peer0.electoralcommission.example.com"),
			TLSCACertPath: os.Getenv("FABRIC_TLS_CA"),
			CertPath:      os.Getenv("FABRIC_CERT"),
			KeyPath:       os.Getenv("FABRIC_KEY"),
			ChannelName:   envOrDefault("FABRIC_CHANNEL", "electionchannel"),
			ChaincodeName: envOrDefault("FABRIC_CHAINCODE", "ballot"),
		},
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
