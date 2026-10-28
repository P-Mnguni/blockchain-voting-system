package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/blockchain-voting-system/pkg/casting"
)

// CastingHandler wraps casting.Service with HTTP endpoints.
type CastingHandler struct {
	Service *casting.Service
}

type castRequest struct {
	ElectionID      string `json:"electionId"`
	EncryptedBallot string `json:"encryptedBallot"`
	Credential      string `json:"credential"`
}

type castResponse struct {
	BallotID    string `json:"ballotId"`
	TxID        string `json:"txId"`
	CommittedAt int64  `json:"committedAt"`
}

func (h *CastingHandler) Cast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req castRequest
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
		msg := err.Error()
		if isClientError(msg) {
			writeError(w, http.StatusForbidden, msg)
		} else {
			log.Printf("[casting] Cast failed: %v", err)
			writeError(w, http.StatusInternalServerError, "cast failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, castResponse{
		BallotID:    receipt.BallotID,
		TxID:        receipt.TxID,
		CommittedAt: receipt.CommittedAt,
	})
}

func (h *CastingHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// isClientError decides whether an error should map to 4xx (client's fault)
// or 5xx (server's fault).
func isClientError(msg string) bool {
	for _, s := range []string{
		"expired",
		"invalid credential",
		"already used",
		"different election",
		"required",
		"malformed",
		"bad payload",
		"bad signature",
		"wrong payload",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
