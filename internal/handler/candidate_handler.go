package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"api/internal/domain"
)

type CandidateHandler struct {
	repo domain.CandidateRepository
}

func NewCandidateHandler(repo domain.CandidateRepository) *CandidateHandler {
	return &CandidateHandler{repo: repo}
}

type candidateSaveRequest struct {
	FileBase64 string `json:"file_base64"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Active     *bool  `json:"active,omitempty"`
}

// SaveProfile guarda o currículo do candidato para o match automático. Responde
// 200 com o perfil salvo (sem o PDF), ou 400/500 com a mensagem de erro.
// @Summary Salva o currículo do candidato para match automático
// @Description Persiste o currículo em base64 junto do e-mail/telefone do candidato. A partir daí, todo flush que indexa vagas novas dispara o match sozinho, sem reenvio do PDF.
// @Tags Candidatos
// @Accept json
// @Produce json
// @Param request body candidateSaveRequest true "Currículo base64 e dados do candidato"
// @Success 200 {object} domain.CandidateProfile "Perfil salvo"
// @Failure 400 {object} domain.ErrorResponse "Requisição inválida"
// @Failure 500 {object} domain.ErrorResponse "Erro ao salvar o perfil"
// @Router /api/v1/candidates [post]
func (h *CandidateHandler) SaveProfile(w http.ResponseWriter, r *http.Request) {
	var req candidateSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	email := strings.TrimSpace(req.Email)
	phone := strings.TrimSpace(req.Phone)
	if email == "" && phone == "" {
		respondWithError(w, http.StatusBadRequest, "At least one of 'email' or 'phone' is required: without it there is no candidate identity nor a channel to apply through")
		return
	}

	if req.FileBase64 == "" {
		respondWithError(w, http.StatusBadRequest, "Field 'file_base64' is required")
		return
	}

	// Currículo inválido aqui vira falha silenciosa lá na frente, dentro de uma
	// goroutine de flush que ninguém está olhando. Barra na entrada.
	resume := normalizeBase64(req.FileBase64)
	if _, err := base64.StdEncoding.DecodeString(resume); err != nil {
		respondWithError(w, http.StatusBadRequest, "Field 'file_base64' is not valid base64: "+err.Error())
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	profile := &domain.CandidateProfile{
		ID:         domain.ProfileID(email, phone),
		Email:      email,
		Phone:      phone,
		ResumeB64:  resume,
		ResumeMime: "application/pdf",
		Active:     active,
	}

	if err := h.repo.SaveProfile(r.Context(), profile); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save candidate profile: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, profile)
}

// ListProfiles lista os perfis com currículo salvo. O PDF não vai na resposta
// (domain.CandidateProfile.ResumeB64 é `json:"-"`).
// @Summary Lista os candidatos com currículo salvo
// @Description Retorna os perfis cadastrados para match automático, sem o conteúdo do currículo.
// @Tags Candidatos
// @Produce json
// @Success 200 {array} domain.CandidateProfile "Perfis cadastrados"
// @Failure 500 {object} domain.ErrorResponse "Erro ao listar perfis"
// @Router /api/v1/candidates [get]
func (h *CandidateHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.repo.ListProfiles(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list candidate profiles: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, profiles)
}

// DeleteProfile remove o perfil identificado por {id}, desligando o match
// automático para ele.
// @Summary Remove o currículo salvo de um candidato
// @Description Remove o perfil pelo ID (o e-mail, ou o telefone quando não há e-mail), desligando o match automático para ele. O histórico de candidaturas já feitas é preservado.
// @Tags Candidatos
// @Produce json
// @Param id path string true "ID do candidato"
// @Success 200 {object} map[string]string "Perfil removido"
// @Failure 400 {object} domain.ErrorResponse "ID é obrigatório"
// @Failure 500 {object} domain.ErrorResponse "Erro ao remover o perfil"
// @Router /api/v1/candidates/{id} [delete]
func (h *CandidateHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondWithError(w, http.StatusBadRequest, "Candidate ID is required")
		return
	}

	if err := h.repo.DeleteProfile(r.Context(), id); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete candidate profile: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Candidate profile deleted successfully",
	})
}
