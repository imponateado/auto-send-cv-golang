package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"api/internal/domain"
)

type ProcessorHandler struct {
	orchestrator domain.Orchestrator
}

// NewProcessorHandler returns a *ProcessorHandler backed by orchestrator.
func NewProcessorHandler(orchestrator domain.Orchestrator) *ProcessorHandler {
	return &ProcessorHandler{
		orchestrator: orchestrator,
	}
}

// Clear apaga todas as vagas do banco vetorial local via h.orchestrator. Responde
// 200 com status "success", ou 500 com a mensagem de erro.
// @Summary Limpa o banco de dados vetorial de vagas
// @Description Apaga todas as vagas do banco vetorial local (chromem-go) e responde com o status da operação.
// @Tags Vagas
// @Produce json
// @Success 200 {object} map[string]string "Banco vetorial de vagas limpo com sucesso"
// @Failure 500 {object} domain.ErrorResponse "Erro interno ao limpar banco vetorial"
// @Router /api/v1/vacancies/clear [post]
func (h *ProcessorHandler) Clear(w http.ResponseWriter, r *http.Request) {
	err := h.orchestrator.ClearVacancies(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to clear vacancies database: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Vacancies database cleared successfully",
	})
}

// List retorna todas as vagas atualmente armazenadas no banco vetorial local via
// h.orchestrator. Responde 200 com a lista de domain.Vacancy, ou 500 com a
// mensagem de erro.
// @Summary Lista todas as vagas do banco vetorial
// @Description Retorna todas as vagas atualmente armazenadas no banco vetorial local (chromem-go).
// @Tags Vagas
// @Produce json
// @Success 200 {array} domain.Vacancy "Lista de vagas"
// @Failure 500 {object} domain.ErrorResponse "Erro interno ao listar vagas"
// @Router /api/v1/vacancies [get]
func (h *ProcessorHandler) List(w http.ResponseWriter, r *http.Request) {
	vacancies, err := h.orchestrator.ListVacancies(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list vacancies: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, vacancies)
}

type matchRequest struct {
	FileBase64     string `json:"file_base64"`
	CandidateEmail string `json:"candidate_email,omitempty"`
	CandidatePhone string `json:"candidate_phone,omitempty"`
}

// Match decodifica o currículo em base64 do corpo, extrai seu texto, busca vagas
// compatíveis e dispara as candidaturas via h.orchestrator. Responde 200 com o
// domain.ProcessResult, ou 400/500 com a mensagem de erro.
// @Summary Executa match de currículo contra banco de vagas
// @Description Decodifica o currículo em base64, extrai seu texto, busca vagas compatíveis via LLM e dispara as candidaturas (e-mail ou WhatsApp) automaticamente.
// @Tags Processador
// @Accept json
// @Produce json
// @Param request body matchRequest true "Payload contendo o currículo base64 e dados do candidato"
// @Success 200 {object} domain.ProcessResult "Resultado da busca vetorial e envios automáticos"
// @Failure 400 {object} domain.ErrorResponse "Requisição inválida"
// @Failure 500 {object} domain.ErrorResponse "Erro ao extrair, buscar ou enviar candidaturas"
// @Router /api/v1/match [post]
func (h *ProcessorHandler) Match(w http.ResponseWriter, r *http.Request) {
	var req matchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if req.FileBase64 == "" {
		respondWithError(w, http.StatusBadRequest, "Field 'file_base64' is required")
		return
	}

	base64Data := req.FileBase64
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}
	base64Data = strings.Join(strings.Fields(base64Data), "")

	res, err := h.orchestrator.MatchResume(r.Context(), base64Data, "application/pdf", req.CandidateEmail, req.CandidatePhone)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to match resume: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, res)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message, "status": "error"})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
