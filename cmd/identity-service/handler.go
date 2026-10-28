package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/blockchain-voting-system/pkg/identity"
)

// Handler wraps identity.Service with HTTP endpoints.
type Handler struct {
	Service *identity.Service
}

// RegisterRequest is the body of POST /register.
type RegisterRequest struct {
	VoterID    string `json:"voterId"`
	ElectionID string `json:"electionId"`
}

// RegisterResponse is the response of POST /register.
type RegisterResponse struct {
	Credential string `json:"credential"`
	Expiry     int64  `json:"expiry"`
}

// Register handles POST /register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	resp, err := h.Service.Register(req.VoterID, req.ElectionID)
	if err != nil {
		var eidErr *identity.EIDError
		var areErr *identity.AlreadyRegisteredError
		switch {
		case errors.As(err, &eidErr):
			writeError(w, http.StatusUnauthorized, eidErr.Error())
		case errors.As(err, &areErr):
			writeError(w, http.StatusConflict, areErr.Error())
		default:
			log.Printf("[identity] Register(%s, %s) failed: %v", req.VoterID, req.ElectionID, err)
			writeError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, RegisterResponse{
		Credential: resp.Credential.Encode(),
		Expiry:     resp.Credential.Expiry.Unix(),
	})
}

// Healthz handles GET /healthz.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
