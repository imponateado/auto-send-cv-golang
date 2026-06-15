package handler

import (
	"encoding/json"
	"net/http"

	"api/internal/domain"
)

type ProcessorHandler struct {
	processor domain.PayloadProcessor
}

// NewProcessorHandler creates a new handler instance.
func NewProcessorHandler(processor domain.PayloadProcessor) *ProcessorHandler {
	return &ProcessorHandler{
		processor: processor,
	}
}

// Process handles POST requests, decodes the JSON body, and calls the service layer.
func (h *ProcessorHandler) Process(w http.ResponseWriter, r *http.Request) {
	var req domain.ProcessRequest

	// Decode the JSON request body
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	// Validate request fields
	if req.Delimiter == "" {
		respondWithError(w, http.StatusBadRequest, "Field 'delimiter' is required and cannot be empty")
		return
	}

	// Call service layer with the request data
	result, err := h.processor.Process(r.Context(), &req)
	if err != nil {
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
