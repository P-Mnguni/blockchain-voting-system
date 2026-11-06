package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/blockchain-voting-system/pkg/fabricclient"
	"github.com/blockchain-voting-system/pkg/tally"
)

// Handler wraps tally.Service with HTTP endpoints.
type Handler struct {
	Service          *tally.Service
	Fabric           fabricclient.Client
	CeremonyDir      string
	PassphrasePrefix string
}

// Tally handles both POST and GET for /tally/{electionID}.
//
//	POST /tally/{id} — run the full ceremony and publish the result
//	GET  /tally/{id} — fetch the published result from the chaincode
func (h *Handler) Tally(w http.ResponseWriter, r *http.Request) {
	electionID := strings.TrimPrefix(r.URL.Path, "/tally/")
	if electionID == "" || strings.Contains(electionID, "/") {
		writeError(w, http.StatusBadRequest, "invalid election ID")
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.runCeremony(w, electionID)
	case http.MethodGet:
		h.getResult(w, electionID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST or GET only")
	}
}

func (h *Handler) runCeremony(w http.ResponseWriter, electionID string) {
	log.Printf("[tally-service] running ceremony for election %q", electionID)

	cfg := tally.Config{
		ElectionID:       electionID,
		CeremonyDir:      h.CeremonyDir,
		PassphrasePrefix: h.PassphrasePrefix,
		Fabric:           h.Fabric,
	}

	result, err := h.Service.Run(cfg)
	if err != nil {
		msg := err.Error()
		if isClientError(msg) {
			log.Printf("[tally-service] client error: %v", err)
			writeError(w, http.StatusBadRequest, msg)
		} else {
			log.Printf("[tally-service] ceremony failed: %v", err)
			writeError(w, http.StatusInternalServerError, "ceremony failed: "+msg)
		}
		return
	}

	writeJSON(w, http.StatusOK, toJSONResponse(result))
}

func (h *Handler) getResult(w http.ResponseWriter, electionID string) {
	result, err := h.Service.GetResult(h.Fabric, electionID)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not published") {
			writeError(w, http.StatusNotFound, msg)
			return
		}
		log.Printf("[tally-service] get result failed: %v", err)
		writeError(w, http.StatusInternalServerError, "get result failed: "+msg)
		return
	}
	writeJSON(w, http.StatusOK, toJSONResponse(result))
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// toJSONResponse converts tally.Result into a JSON-friendly shape.
// The zkproofs.DecryptionProof has custom JSON marshaling that serializes
// big.Int fields as decimal strings, so it round-trips cleanly.
func toJSONResponse(r *tally.Result) interface{} {
	return r
}

// isClientError decides whether an error should map to 4xx (client's fault)
// or 5xx (server's fault).
func isClientError(msg string) bool {
	for _, s := range []string{
		"required",
		"no ballots",
		"not found",
		"not published",
		"invalid",
		"not in FROZEN",
		"not in VOTING",
		"not accepting ballots",
		"already published",
		"already tallied",
		"need ",
		"missing",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ensure errors package is used (import guard)
var _ = errors.New
