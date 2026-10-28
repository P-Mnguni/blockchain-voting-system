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

	"github.com/blockchain-voting-system/pkg/config"
	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/identity"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[identity] %v", err)
	}
}

func run() error {
	log.SetPrefix("[identity] ")

	cfg, err := config.LoadIdentity()
	if err != nil {
		return err
	}

	// Generate the issuer key pair. In production this comes from an HSM;
	// for Milestone 3 it's generated in-memory at startup.
	issuer, err := blind.NewBlindSignatureIssuer(2048)
	if err != nil {
		return fmt.Errorf("create issuer: %w", err)
	}

	// Connect to Fabric. We use the Fabric client only for future features
	// (e.g., checking the election exists). For now, we still validate that
	// connectivity works at startup.
	fabricCfg := fabricclient.Config{
		MSPID:         cfg.Fabric.MSPID,
		PeerEndpoint:  cfg.Fabric.PeerEndpoint,
		PeerHostname:  cfg.Fabric.PeerHostname,
		TLSCACertPath: cfg.Fabric.TLSCACertPath,
		CertPath:      cfg.Fabric.CertPath,
		KeyPath:       cfg.Fabric.KeyPath,
		ChannelName:   cfg.Fabric.ChannelName,
		ChaincodeName: cfg.Fabric.ChaincodeName,
	}
	fab, err := fabricclient.Connect(fabricCfg)
	if err != nil {
		return fmt.Errorf("connect fabric: %w", err)
	}
	defer fab.Close()

	svc := &identity.Service{
		Issuer:        issuer,
		EID:           identity.NewMockEID(),
		Voters:        identity.NewMemoryVoterStore(),
		CredentialTTL: cfg.CredentialTTL,
	}

	handler := &Handler{Service: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("/register", handler.Register)
	mux.HandleFunc("/healthz", handler.Healthz)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("listening on :%s (election=%s)", cfg.Port, cfg.ElectionID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-stop
	log.Print("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
