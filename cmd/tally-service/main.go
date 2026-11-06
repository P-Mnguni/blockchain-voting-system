package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/tally"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[tally-service] %v", err)
	}
}

func run() error {
	log.SetPrefix("[tally-service] ")

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Connect to Fabric
	fabCfg := fabricclient.Config{
		MSPID:         cfg.FabricMSPID,
		PeerEndpoint:  cfg.FabricEndpoint,
		PeerHostname:  cfg.FabricHostname,
		TLSCACertPath: cfg.FabricTLSCA,
		CertPath:      cfg.FabricCert,
		KeyPath:       cfg.FabricKey,
		ChannelName:   cfg.FabricChannel,
		ChaincodeName: cfg.FabricChaincode,
	}
	fab, err := fabricclient.Connect(fabCfg)
	if err != nil {
		return fmt.Errorf("connect fabric: %w", err)
	}
	defer fab.Close()

	svc := &tally.Service{}
	handler := &Handler{
		Service:          svc,
		Fabric:           fab,
		CeremonyDir:      cfg.CeremonyDir,
		PassphrasePrefix: cfg.PassphrasePrefix,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tally/", handler.Tally) // POST /tally/{electionID}, GET /tally/{electionID}
	mux.HandleFunc("/healthz", handler.Healthz)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 300 * time.Second, // tally ceremony can take minutes
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("listening on :%s (ceremony=%s)", cfg.Port, cfg.CeremonyDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-stop
	log.Print("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

type config struct {
	Port             string
	CeremonyDir      string
	PassphrasePrefix string

	FabricMSPID     string
	FabricEndpoint  string
	FabricHostname  string
	FabricTLSCA     string
	FabricCert      string
	FabricKey       string
	FabricChannel   string
	FabricChaincode string
}

func loadConfig() (*config, error) {
	cfg := &config{
		Port:             envOrDefault("TALLY_PORT", "8082"),
		CeremonyDir:      envOrDefault("CEREMONY_DIR", tally.DefaultCeremonyDir),
		PassphrasePrefix: envOrDefault("PASSPHRASE_PREFIX", tally.DefaultPassphrasePrefix),

		FabricMSPID:     os.Getenv("FABRIC_MSP_ID"),
		FabricEndpoint:  os.Getenv("FABRIC_ENDPOINT"),
		FabricHostname:  envOrDefault("FABRIC_HOSTNAME", "peer0.electoralcommission.example.com"),
		FabricTLSCA:     os.Getenv("FABRIC_TLS_CA"),
		FabricCert:      os.Getenv("FABRIC_CERT"),
		FabricKey:       os.Getenv("FABRIC_KEY"),
		FabricChannel:   envOrDefault("FABRIC_CHANNEL", "electionchannel"),
		FabricChaincode: envOrDefault("FABRIC_CHAINCODE", "ballot"),
	}

	if cfg.FabricMSPID == "" {
		return nil, fmt.Errorf("FABRIC_MSP_ID is required")
	}
	if cfg.FabricEndpoint == "" {
		return nil, fmt.Errorf("FABRIC_ENDPOINT is required")
	}
	if cfg.FabricTLSCA == "" {
		return nil, fmt.Errorf("FABRIC_TLS_CA is required")
	}
	if cfg.FabricCert == "" {
		return nil, fmt.Errorf("FABRIC_CERT is required")
	}
	if cfg.FabricKey == "" {
		return nil, fmt.Errorf("FABRIC_KEY is required")
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
