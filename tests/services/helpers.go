package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/blockchain-voting-system/pkg/fabricclient"
)

// Env returns a required environment variable or skips the test.
func Env(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("set %s to run integration tests", key)
	}
	return v
}

// HTTPClient is a small wrapper for JSON requests.
type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *HTTPClient) PostJSON(path string, body interface{}, out interface{}) (int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	resp, err := c.client.Post(c.baseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w (body: %s)", err, raw)
		}
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return resp.StatusCode, nil
}

func (c *HTTPClient) Get(path string) (int, error) {
	resp, err := c.client.Get(c.baseURL + path)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// FabricConfigFromEnv builds a fabricclient.Config from the same env vars the
// voting service uses. All are optional in tests — if any are missing, the
// caller should skip.
func FabricConfigFromEnv(t *testing.T) fabricclient.Config {
	t.Helper()
	return fabricclient.Config{
		MSPID:         Env(t, "FABRIC_MSP_ID"),
		PeerEndpoint:  Env(t, "FABRIC_ENDPOINT"),
		PeerHostname:  Env(t, "FABRIC_HOSTNAME"),
		TLSCACertPath: Env(t, "FABRIC_TLS_CA"),
		CertPath:      Env(t, "FABRIC_CERT"),
		KeyPath:       Env(t, "FABRIC_KEY"),
		ChannelName:   envOrDefault("FABRIC_CHANNEL", "electionchannel"),
		ChaincodeName: envOrDefault("FABRIC_CHAINCODE", "ballot"),
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
