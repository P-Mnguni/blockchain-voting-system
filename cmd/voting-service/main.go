package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/blockchain-voting-system/pkg/casting"
	"github.com/blockchain-voting-system/pkg/config"
	"github.com/blockchain-voting-system/pkg/crypto/blind"
	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/identity"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[voting] %v", err)
	}
}

func run() error {
	log.SetPrefix("[voting] ")

	cfg, err := config.LoadVoting()
	if err != nil {
		return err
	}

	// One issuer shared by both services. This is why we run them in the same
	// process for Milestone 3B — no key serialization needed. Milestone 4
	// splits them with a proper key exchange.
	issuer, err := blind.NewBlindSignatureIssuer(2048)
	if err != nil {
		return fmt.Errorf("create issuer: %w", err)
	}

	fabCfg := fabricclient.Config{
		MSPID:         cfg.Fabric.MSPID,
		PeerEndpoint:  cfg.Fabric.PeerEndpoint,
		PeerHostname:  cfg.Fabric.PeerHostname,
		TLSCACertPath: cfg.Fabric.TLSCACertPath,
		CertPath:      cfg.Fabric.CertPath,
		KeyPath:       cfg.Fabric.KeyPath,
		ChannelName:   cfg.Fabric.ChannelName,
		ChaincodeName: cfg.Fabric.ChaincodeName,
	}
	fab, err := fabricclient.Connect(fabCfg)
	if err != nil {
		return fmt.Errorf("connect fabric: %w", err)
	}
	defer fab.Close()

	// Core services
	identitySvc := &identity.Service{
		Issuer:        issuer,
		EID:           identity.NewMockEID(),
		Voters:        identity.NewMemoryVoterStore(),
		CredentialTTL: cfg.CredentialTTL,
	}
	castingSvc := &casting.Service{
		Fabric:     fab,
		Issuer:     issuer,
		ElectionID: cfg.ElectionID,
	}

	// HTTP handlers
	idHandler := &IdentityHandler{Service: identitySvc}
	castHandler := &CastingHandler{Service: castingSvc}

	idMux := http.NewServeMux()
	idMux.HandleFunc("/register", idHandler.Register)
	idMux.HandleFunc("/healthz", idHandler.Healthz)

	castMux := http.NewServeMux()
	castMux.HandleFunc("/cast", castHandler.Cast)
	castMux.HandleFunc("/healthz", castHandler.Healthz)

	idSrv := &http.Server{
		Addr:         ":" + cfg.IdentityPort,
		Handler:      idMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	castSrv := &http.Server{
		Addr:         ":" + cfg.CastingPort,
		Handler:      castMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start both servers
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	startServer := func(srv *http.Server, name string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("%s listening on %s (election=%s)", name, srv.Addr, cfg.ElectionID)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("%s: %w", name, err)
			}
		}()
	}
	startServer(idSrv, "identity")
	startServer(castSrv, "casting")

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		log.Print("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := idSrv.Shutdown(ctx); err != nil {
			log.Printf("identity shutdown: %v", err)
		}
		if err := castSrv.Shutdown(ctx); err != nil {
			log.Printf("casting shutdown: %v", err)
		}
		wg.Wait()
		return nil
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
