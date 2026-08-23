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
	Provider     string `json:"provider"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// RegisterCredentials decodifica e valida as credenciais OAuth2 do corpo da
// requisição e as persiste via h.credsRepo. Responde 200 com status "success" ou
// 400/500 com a mensagem de erro.
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

// DeleteCredentials remove as credenciais de e-mail do candidato identificado
// pelo parâmetro de rota {email}. Responde 200 com status "success" ou 400/500
// com a mensagem de erro.
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

type credentialSummary struct {
	Email    string `json:"email"`
	Provider string `json:"provider"`
}

// ListCredentials lista as credenciais de e-mail cadastradas via h.credsRepo, sem expor refresh_token/client_secret. Responde 200 com a lista, ou 500 com a msg de erro.
func (h *CredentialsHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := h.credsRepo.ListEmailCredentials(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list credentials: "+err.Error())
		return
	}

	summaries := make([]credentialSummary, len(creds))
	for i, c := range creds {
		summaries[i] = credentialSummary{Email: c.Email, Provider: c.Provider}
	}

	respondWithJSON(w, http.StatusOK, summaries)
}

// GetCredentials consulta as credenciais do candidato identificado pelo parâmetro de rota {email}, sem expor refresh_token/client_secret. Responde 200 com o resumo, ou 400/404 com msg de erro.
func (h *CredentialsHandler) GetCredentials(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	if email == "" {
		respondWithError(w, http.StatusBadRequest, "Email parameter is required")
		return
	}

	creds, err := h.credsRepo.GetEmailCredentials(r.Context(), email)
	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, credentialSummary{Email: creds.Email, Provider: creds.Provider})
}

// GetWhatsAppQR gera (ou recupera) o QR code de pareamento para o número em
// ?phone=. Responde com a imagem PNG do QR, ou 200/success se já autenticado, ou
// 400/500 com a mensagem de erro.
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
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(qrBytes)
}

// GetWhatsAppStatus consulta o status de conexão do número em ?phone=. Responde
// 200 com o domain.WhatsAppStatus, ou 400/500 com a mensagem de erro.
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

// ListWhatsAppConnections lista todo número que já completou pareamento com o
// WhatsApp. Responde 200 com a lista de domain.WhatsAppStatus, ou 500 com a
// mensagem de erro.
func (h *CredentialsHandler) ListWhatsAppConnections(w http.ResponseWriter, r *http.Request) {
	statuses, err := h.waManager.ListConnected(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list connections: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, statuses)
}

// DisconnectWhatsApp desloga e apaga a sessão do WhatsApp do número em ?phone=.
// Responde 200 com status "success", ou 400/500 com a mensagem de erro.
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
