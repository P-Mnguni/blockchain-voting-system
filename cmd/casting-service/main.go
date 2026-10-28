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

	"github.com/blockchain-voting-system/pkg/casting"
	"github.com/blockchain-voting-system/pkg/config"
	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/fabricclient"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[casting] %v", err)
	}
}

func run() error {
	log.SetPrefix("[casting] ")

	cfg, err := config.LoadCasting()
	if err != nil {
		return err
	}

	// The casting service only needs the issuer's PUBLIC key to verify
	// credentials. In Milestone 3 we generate a fresh issuer here as a
	// placeholder — but the real deployment loads the public key from a
	// shared source. See "Shared issuer key" note below.
	issuer, err := blind.NewBlindSignatureIssuer(2048)
	if err != nil {
		return fmt.Errorf("create issuer verifier: %w", err)
	}

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

	svc := &casting.Service{
		Fabric:     fab,
		Issuer:     issuer,
		ElectionID: cfg.ElectionID,
	}

	handler := &Handler{Service: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("/cast", handler.Cast)
	mux.HandleFunc("/healthz", handler.Healthz)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

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
