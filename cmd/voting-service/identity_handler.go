package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/blockchain-voting-system/pkg/identity"
)

// IdentityHandler wraps identity.Service with HTTP endpoints.
type IdentityHandler struct {
	Service *identity.Service
}

type registerRequest struct {
	VoterID    string `json:"voterId"`
	ElectionID string `json:"electionId"`
}

type registerResponse struct {
	Credential string `json:"credential"`
	Expiry     int64  `json:"expiry"`
}

func (h *IdentityHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req registerRequest
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
			log.Printf("[identity] Register failed: %v", err)
			writeError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, registerResponse{
		Credential: resp.Credential.Encode(),
		Expiry:     resp.Credential.Expiry.Unix(),
	})
}

func (h *IdentityHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
