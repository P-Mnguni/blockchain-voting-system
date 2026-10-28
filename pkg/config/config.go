// Package config loads service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Identity holds configuration for the identity service.
type Identity struct {
	Port          string
	Fabric        Fabric
	ElectionID    string
	CredentialTTL time.Duration
}

// Casting holds configuration for the casting service.
type Casting struct {
	Port       string
	Fabric     Fabric
	ElectionID string
}

// Fabric holds the Gateway SDK configuration shared by both services.
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

// LoadIdentity reads the identity service config from env vars.
func LoadIdentity() (Identity, error) {
	fabric, err := loadFabric()
	if err != nil {
		return Identity{}, err
	}

	electionID := os.Getenv("ELECTION_ID")
	if electionID == "" {
		return Identity{}, fmt.Errorf("ELECTION_ID is required")
	}

	ttl := 24 * time.Hour
	if v := os.Getenv("CREDENTIAL_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return Identity{}, fmt.Errorf("CREDENTIAL_TTL: %w", err)
		}
		ttl = parsed
	}

	return Identity{
		Port:          envOrDefault("IDENTITY_PORT", "8080"),
		Fabric:        fabric,
		ElectionID:    electionID,
		CredentialTTL: ttl,
	}, nil
}

// LoadCasting reads the casting service config from env vars.
func LoadCasting() (Casting, error) {
	fabric, err := loadFabric()
	if err != nil {
		return Casting{}, err
	}

	electionID := os.Getenv("ELECTION_ID")
	if electionID == "" {
		return Casting{}, fmt.Errorf("ELECTION_ID is required")
	}

	return Casting{
		Port:       envOrDefault("CASTING_PORT", "8081"),
		Fabric:     fabric,
		ElectionID: electionID,
	}, nil
}

func loadFabric() (Fabric, error) {
	required := []string{
		"FABRIC_MSP_ID",
		"FABRIC_ENDPOINT",
		"FABRIC_TLS_CA",
		"FABRIC_CERT",
		"FABRIC_KEY",
	}
	for _, k := range required {
		if os.Getenv(k) == "" {
			return Fabric{}, fmt.Errorf("%s is required", k)
		}
	}

	return Fabric{
		MSPID:         os.Getenv("FABRIC_MSP_ID"),
		PeerEndpoint:  os.Getenv("FABRIC_ENDPOINT"),
		PeerHostname:  envOrDefault("FABRIC_HOSTNAME", "peer0.electoralcommission.example.com"),
		TLSCACertPath: os.Getenv("FABRIC_TLS_CA"),
		CertPath:      os.Getenv("FABRIC_CERT"),
		KeyPath:       os.Getenv("FABRIC_KEY"),
		ChannelName:   envOrDefault("FABRIC_CHANNEL", "electionchannel"),
		ChaincodeName: envOrDefault("FABRIC_CHAINCODE", "ballot"),
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// MustInt parses an integer env var or returns the default.
func MustInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
