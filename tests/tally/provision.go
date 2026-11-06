package tally

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blockchain-voting-system/pkg/crypto/elgamal"
	"github.com/blockchain-voting-system/pkg/keyceremony"
)

// CeremonyDir returns the host path where the ceremony will be generated.
// This is a subdirectory of deployments/fabric/ceremony/ because that's what
// is mounted into the tally-service container as /ceremony.
func CeremonyDir(runID string) string {
	return filepath.Join("/work/deployments/fabric/ceremony", runID)
}

// ContainerCeremonyDir returns the same path as seen from inside the
// tally-service container.
func ContainerCeremonyDir(runID string) string {
	return filepath.Join("/ceremony", runID)
}

// ProvisionCeremony generates a fresh 3-of-5 ceremony on disk.
// Returns the host-side ceremony directory and the matching public key.
func ProvisionCeremony(t *testing.T, runID string) (string, *elgamal.PublicKey) {
	t.Helper()

	dir := CeremonyDir(runID)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("clean ceremony dir: %v", err)
	}

	result, err := keyceremony.RunTrustedDealer(keyceremony.Config{
		OutputDir:        dir,
		Threshold:        3,
		Total:            5,
		KeySizeBits:      1024, // smaller for test speed
		PassphrasePrefix: "dev-keyholder-",
	})
	if err != nil {
		t.Fatalf("RunTrustedDealer failed: %v", err)
	}

	t.Logf("ceremony generated at %s (threshold=3, total=5)", dir)
	return dir, result.PublicKey
}

// ProvisionElection registers and starts an election with the given
// homomorphic public key. Returns the election ID.
func ProvisionElection(t *testing.T, fab *FabricClient, pubKey *elgamal.PublicKey) string {
	t.Helper()

	electionID := fmt.Sprintf("tally-test-%d", time.Now().UnixNano())
	candidatesJSON := `["Alice","Bob"]`
	pubKeyJSON := serializePublicKey(t, pubKey)

	fab.Submit(t, "RegisterElection", electionID, "Tally E2E Test", candidatesJSON, pubKeyJSON)
	fab.Submit(t, "StartVoting", electionID)

	t.Logf("election provisioned: %s", electionID)
	return electionID
}

func serializePublicKey(t *testing.T, pk *elgamal.PublicKey) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{
		"G": pk.G.String(),
		"P": pk.P.String(),
		"Y": pk.Y.String(),
	})
	if err != nil {
		t.Fatalf("serialize public key: %v", err)
	}
	return string(b)
}
