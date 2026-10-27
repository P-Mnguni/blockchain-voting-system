// Package fabricclient is a thin wrapper around the Hyperledger Fabric Gateway
// SDK. It exposes just what the identity and casting services need: submit a
// transaction, evaluate a transaction. Nothing more.
package fabricclient

import (
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Client is the interface both services depend on. Mock it for unit tests.
type Client interface {
	Submit(fn string, args ...string) ([]byte, error)
	Evaluate(fn string, args ...string) ([]byte, error)
	Close() error
}

// Config holds everything needed to connect to a Fabric peer via the Gateway SDK.
type Config struct {
	MSPID         string
	PeerEndpoint  string // host:port
	PeerHostname  string // TLS server name (matches cert SAN)
	TLSCACertPath string
	CertPath      string
	KeyPath       string
	ChannelName   string
	ChaincodeName string
	Timeout       time.Duration // optional, defaults to 30s
}

// gatewayClient is the real implementation.
type gatewayClient struct {
	gw       *client.Gateway
	contract *client.Contract
	conn     *grpc.ClientConn
}

// Connect establishes a Gateway connection. Caller must call Close().
func Connect(cfg Config) (Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	caPEM, err := os.ReadFile(cfg.TLSCACertPath)
	if err != nil {
		return nil, fmt.Errorf("read TLS CA cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to add TLS CA cert to pool")
	}
	creds := credentials.NewClientTLSFromCert(pool, cfg.PeerHostname)

	// passthrough:/// is required for gRPC >=1.63 with bare hostnames.
	target := "passthrough:///" + cfg.PeerEndpoint
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}

	certPEM, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read user cert: %w", err)
	}
	certificate, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse user cert: %w", err)
	}
	id, err := identity.NewX509Identity(cfg.MSPID, certificate)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("build X509 identity: %w", err)
	}

	keyPEM, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read user key: %w", err)
	}
	privateKey, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse user key: %w", err)
	}
	sign, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("build signer: %w", err)
	}

	gw, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(5*time.Second),
		client.WithEndorseTimeout(cfg.Timeout),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(cfg.Timeout),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("gateway connect: %w", err)
	}

	contract := gw.GetNetwork(cfg.ChannelName).GetContract(cfg.ChaincodeName)

	return &gatewayClient{gw: gw, contract: contract, conn: conn}, nil
}

func (g *gatewayClient) Submit(fn string, args ...string) ([]byte, error) {
	return g.contract.SubmitTransaction(fn, args...)
}

func (g *gatewayClient) Evaluate(fn string, args ...string) ([]byte, error) {
	return g.contract.EvaluateTransaction(fn, args...)
}

func (g *gatewayClient) Close() error {
	if g.gw != nil {
		g.gw.Close()
	}
	if g.conn != nil {
		return g.conn.Close()
	}
	return nil
}
