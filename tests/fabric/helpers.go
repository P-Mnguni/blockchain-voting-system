package fabric

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// NetworkConfig holds the paths and endpoints needed to connect to the
// dev Fabric network. The defaults match deployments/fabric/crypto-config.
type NetworkConfig struct {
	MSPID         string
	PeerEndpoint  string // e.g. "localhost:7051"
	PeerHostname  string // TLS server name, e.g. "peer0.electoralcommission.example.com"
	TLSCACertPath string
	CertPath      string // user signing cert (PEM)
	KeyPath       string // user private key (PEM, PKCS8 or SEC1)
	ChannelName   string
	ChaincodeName string
}

// DefaultConfig returns the config that matches the dev network in
// deployments/fabric/.
func DefaultConfig() NetworkConfig {
	base := "../../deployments/fabric/crypto-config"
	ecOrg := filepath.Join(base, "peerOrganizations/electoralcommission.example.com")
	userMSP := filepath.Join(ecOrg, "users/Admin@electoralcommission.example.com/msp")

	return NetworkConfig{
		MSPID:         "ElectoralCommissionMSP",
		PeerEndpoint:  "peer0.electoralcommission.example.com:7051",
		PeerHostname:  "peer0.electoralcommission.example.com",
		TLSCACertPath: filepath.Join(ecOrg, "peers/peer0.electoralcommission.example.com/tls/ca.crt"),
		CertPath:      filepath.Join(userMSP, "signcerts/Admin@electoralcommission.example.com-cert.pem"),
		KeyPath:       findKeyPath(userMSP),
		ChannelName:   "electionchannel",
		ChaincodeName: "ballot",
	}
}

// findKeyPath locates the single PEM private key inside the user's keystore
// directory. Cryptogen names the file "priv_sk"; other tooling may use
// _sk. We just take the first file we find.
func findKeyPath(mspDir string) string {
	keystore := filepath.Join(mspDir, "keystore")
	entries, err := os.ReadDir(keystore)
	if err != nil || len(entries) == 0 {
		return ""
	}
	return filepath.Join(keystore, entries[0].Name())
}

// Gateway bundles the client, contract, and connection so tests can clean up
// at the end of a run.
type Gateway struct {
	Client   *client.Gateway
	Contract *client.Contract
	Conn     *grpc.ClientConn
}

// Connect establishes a Gateway connection to the peer. Caller must call Close().
func Connect(cfg NetworkConfig) (*Gateway, error) {
	// Load the TLS CA cert for the peer connection
	caPEM, err := os.ReadFile(cfg.TLSCACertPath)
	if err != nil {
		return nil, fmt.Errorf("read TLS CA cert: %w", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to add TLS CA cert to pool")
	}

	// grpc creds: connect over TLS but override server name so the cert's
	// SAN (peer0.electoralcommission.example.com) matches even though we
	// connect via localhost.
	creds := credentials.NewClientTLSFromCert(certPool, cfg.PeerHostname)

	target := "passthrough:///" + cfg.PeerEndpoint
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}

	// Load the user identity
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

	// Load the private key
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
		client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(60*time.Second),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("gateway connect: %w", err)
	}

	network := gw.GetNetwork(cfg.ChannelName)
	contract := network.GetContract(cfg.ChaincodeName)

	return &Gateway{
		Client:   gw,
		Contract: contract,
		Conn:     conn,
	}, nil
}

// Close releases resources.
func (g *Gateway) Close() {
	if g.Client != nil {
		g.Client.Close()
	}
	if g.Conn != nil {
		g.Conn.Close()
	}
}
