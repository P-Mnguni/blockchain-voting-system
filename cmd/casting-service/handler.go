package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/blockchain-voting-system/pkg/casting"
)

// Handler wraps casting.Service with HTTP endpoints.
type Handler struct {
	Service *casting.Service
}

// CastRequest is the body of POST /cast.
type CastRequest struct {
	ElectionID      string `json:"electionId"`
	EncryptedBallot string `json:"encryptedBallot"`
	Credential      string `json:"credential"`
}

// CastResponse is the response of POST /cast.
type CastResponse struct {
	BallotID    string `json:"ballotId"`
	TxID        string `json:"txId"`
	CommittedAt int64  `json:"committedAt"`
}

// Cast handles POST /cast.
func (h *Handler) Cast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req CastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.ElectionID != "" && req.ElectionID != h.Service.ElectionID {
		writeError(w, http.StatusBadRequest, "election ID mismatch")
		return
	}

	receipt, err := h.Service.Cast(casting.CastRequest{
		EncryptedBallot: req.EncryptedBallot,
		Credential:      req.Credential,
	})
	if err != nil {
		// Classify errors for better HTTP semantics
		msg := err.Error()
		switch {
		case contains(msg, "expired"),
			contains(msg, "invalid credential"),
			contains(msg, "already used"),
			contains(msg, "different election"):
			writeError(w, http.StatusForbidden, msg)
		default:
			log.Printf("[casting] Cast failed: %v", err)
			writeError(w, http.StatusInternalServerError, "cast failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, CastResponse{
		BallotID:    receipt.BallotID,
		TxID:        receipt.TxID,
		CommittedAt: receipt.CommittedAt,
	})
}

// Healthz handles GET /healthz.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
