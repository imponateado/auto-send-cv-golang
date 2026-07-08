package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"api/internal/domain"
	"api/internal/infra/whatsapp"
)

type CredentialsHandler struct {
	credsRepo domain.CredentialsRepository
	waManager *whatsapp.WhatsMeowManager
}

func NewCredentialsHandler(credsRepo domain.CredentialsRepository, waManager *whatsapp.WhatsMeowManager) *CredentialsHandler {
	return &CredentialsHandler{
		credsRepo: credsRepo,
		waManager: waManager,
	}
}

type credentialsRegisterRequest struct {
	Email        string `json:"email"`
	Provider     string `json:"provider"` // "google" ou "microsoft"
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// RegisterCredentials gerencia POST /api/v1/credentials
func (h *CredentialsHandler) RegisterCredentials(w http.ResponseWriter, r *http.Request) {
	var req credentialsRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if req.Email == "" || req.Provider == "" || req.RefreshToken == "" || req.ClientID == "" || req.ClientSecret == "" {
		respondWithError(w, http.StatusBadRequest, "Fields 'email', 'provider', 'refresh_token', 'client_id', and 'client_secret' are all required")
		return
	}

	prov := strings.ToLower(req.Provider)
	if prov != "google" && prov != "microsoft" {
		respondWithError(w, http.StatusBadRequest, "Provider must be either 'google' or 'microsoft'")
		return
	}

	creds := &domain.EmailCredentials{
		Email:        req.Email,
		Provider:     prov,
		RefreshToken: req.RefreshToken,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
	}

	err := h.credsRepo.SaveEmailCredentials(r.Context(), creds)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save credentials: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Credentials registered successfully",
	})
}

// DeleteCredentials gerencia DELETE /api/v1/credentials/{email}
func (h *CredentialsHandler) DeleteCredentials(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	if email == "" {
		respondWithError(w, http.StatusBadRequest, "Email parameter is required")
		return
	}

	err := h.credsRepo.DeleteEmailCredentials(r.Context(), email)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete credentials: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Credentials deleted successfully",
	})
}

// GetWhatsAppQR gerencia GET /api/v1/whatsapp/qr?phone=...
func (h *CredentialsHandler) GetWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respondWithError(w, http.StatusBadRequest, "Query parameter 'phone' is required")
		return
	}

	qrBytes, err := h.waManager.GetQR(r.Context(), phone)
	if err != nil {
		if err.Error() == "already authenticated" {
			respondWithJSON(w, http.StatusOK, map[string]string{
				"status":  "success",
				"message": "WhatsApp is already connected for this phone",
			})
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to generate QR code: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(qrBytes)
}

// GetWhatsAppStatus gerencia GET /api/v1/whatsapp/status?phone=...
func (h *CredentialsHandler) GetWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respondWithError(w, http.StatusBadRequest, "Query parameter 'phone' is required")
		return
	}

	status, err := h.waManager.GetStatus(r.Context(), phone)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to check status: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, status)
}

// DisconnectWhatsApp gerencia POST /api/v1/whatsapp/disconnect?phone=...
func (h *CredentialsHandler) DisconnectWhatsApp(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respondWithError(w, http.StatusBadRequest, "Query parameter 'phone' is required")
		return
	}

	err := h.waManager.Disconnect(r.Context(), phone)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to disconnect: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "WhatsApp disconnected successfully",
	})
}
