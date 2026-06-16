package handler

import (
	"encoding/json"
	"net/http"

	"api/internal/domain"
)

type ProcessorHandler struct {
	orchestrator domain.Orchestrator
}

// NewProcessorHandler creates a new handler instance.
func NewProcessorHandler(orchestrator domain.Orchestrator) *ProcessorHandler {
	return &ProcessorHandler{
		orchestrator: orchestrator,
	}
}

// Process handles POST requests, decodes the JSON body, and calls the orchestrator layer.
func (h *ProcessorHandler) Process(w http.ResponseWriter, r *http.Request) {
	var req domain.ProcessRequest

	// Decode the JSON request body
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	// Validate request fields
	if req.Content == "" && req.FileBase64 == "" {
		respondWithError(w, http.StatusBadRequest, "Either 'content' or 'file_base64' must be provided")
		return
	}

	if req.Content != "" && req.Delimiter == "" {
		respondWithError(w, http.StatusBadRequest, "Field 'delimiter' is required when 'content' is provided")
		return
	}

	// Call orchestrator with the request data
	result, err := h.orchestrator.RunMatchAndDispatch(r.Context(), &req)
	if err != nil {
		// Distinguish bad base64 encoding as a Bad Request (400)
		if err.Error() == "invalid base64 encoding" {
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to process payload: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, result)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message, "status": "error"})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
