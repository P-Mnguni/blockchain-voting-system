package tally

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func Env(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("set %s to run integration tests", key)
	}
	return v
}

type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 300 * time.Second},
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

func (c *HTTPClient) Get(path string, out interface{}) (int, error) {
	resp, err := c.client.Get(c.baseURL + path)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w (body: %s)", err, raw)
		}
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return resp.StatusCode, nil
}

// --- Fabric client (read-only + provisioning) ---

type FabricClient struct {
	gw       *client.Gateway
	contract *client.Contract
	conn     *grpc.ClientConn
}

func NewFabricClient(t *testing.T) *FabricClient {
	t.Helper()

	mspID := Env(t, "FABRIC_MSP_ID")
	endpoint := Env(t, "FABRIC_ENDPOINT")
	hostname := Env(t, "FABRIC_HOSTNAME")
	caPath := Env(t, "FABRIC_TLS_CA")
	certPath := Env(t, "FABRIC_CERT")
	keyPath := Env(t, "FABRIC_KEY")
	channel := Env(t, "FABRIC_CHANNEL")
	chaincode := Env(t, "FABRIC_CHAINCODE")

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("read CA: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to add CA to pool")
	}
	creds := credentials.NewClientTLSFromCert(pool, hostname)

	target := "passthrough:///" + endpoint
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		conn.Close()
		t.Fatalf("read cert: %v", err)
	}
	certificate, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		conn.Close()
		t.Fatalf("parse cert: %v", err)
	}
	id, err := identity.NewX509Identity(mspID, certificate)
	if err != nil {
		conn.Close()
		t.Fatalf("build identity: %v", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		conn.Close()
		t.Fatalf("read key: %v", err)
	}
	privateKey, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		conn.Close()
		t.Fatalf("parse key: %v", err)
	}
	sign, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		conn.Close()
		t.Fatalf("build signer: %v", err)
	}

	gw, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(30*time.Second),
		client.WithEndorseTimeout(120*time.Second),
		client.WithSubmitTimeout(30*time.Second),
		client.WithCommitStatusTimeout(120*time.Second),
	)
	if err != nil {
		conn.Close()
		t.Fatalf("gateway connect: %v", err)
	}

	return &FabricClient{
		gw:       gw,
		contract: gw.GetNetwork(channel).GetContract(chaincode),
		conn:     conn,
	}
}

func (f *FabricClient) Close() {
	if f.gw != nil {
		f.gw.Close()
	}
	if f.conn != nil {
		f.conn.Close()
	}
}

func (f *FabricClient) Evaluate(t *testing.T, fn string, args ...string) []byte {
	t.Helper()
	out, err := f.contract.EvaluateTransaction(fn, args...)
	if err != nil {
		t.Fatalf("Evaluate(%s) failed: %v", fn, err)
	}
	return out
}

func (f *FabricClient) Submit(t *testing.T, fn string, args ...string) []byte {
	t.Helper()
	out, err := f.contract.SubmitTransaction(fn, args...)
	if err != nil {
		t.Fatalf("Submit(%s) failed: %v", fn, err)
	}
	return out
}
