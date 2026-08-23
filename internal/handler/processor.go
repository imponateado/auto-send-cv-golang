package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

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

// DeleteVacancy remove uma vaga específica do banco vetorial local via
// h.orchestrator, identificada pelo parâmetro de rota {id}. Responde 200 com
// status "success", ou 400/500 com a mensagem de erro.
// @Summary Remove uma vaga específica do banco vetorial
// @Description Remove a vaga identificada pelo ID (retornado em cada item da listagem) do banco vetorial local (chromem-go).
// @Tags Vagas
// @Produce json
// @Param id path string true "ID da vaga"
// @Success 200 {object} map[string]string "Vaga removida com sucesso"
// @Failure 400 {object} domain.ErrorResponse "ID da vaga é obrigatório"
// @Failure 500 {object} domain.ErrorResponse "Erro interno ao remover vaga"
// @Router /api/v1/vacancies/{id} [delete]
func (h *ProcessorHandler) DeleteVacancy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondWithError(w, http.StatusBadRequest, "Vacancy ID is required")
		return
	}

	if err := h.orchestrator.DeleteVacancy(r.Context(), id); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete vacancy: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Vacancy deleted successfully",
	})
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

	// Disparo de candidatura é efeito colateral externo: uma vez começado, não
	// pode morrer no meio porque o cliente HTTP desistiu. WithoutCancel mantém os
	// valores do contexto e descarta só o cancelamento.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	defer cancel()

	res, err := h.orchestrator.MatchResume(ctx, base64Data, "application/pdf", req.CandidateEmail, req.CandidatePhone)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to match resume: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, res)
}

// ListMatches lista o histórico de execuções de match desta sessão do servidor
// (em memória, perdido num restart) via h.orchestrator. Responde 200 com a
// lista de domain.MatchRecord, ou 500 com a mensagem de erro.
// @Summary Lista o histórico de execuções de match
// @Description Retorna o histórico (em memória, não persistido) de execuções de match desta sessão do servidor.
// @Tags Processador
// @Produce json
// @Success 200 {array} domain.MatchRecord "Histórico de matches"
// @Failure 500 {object} domain.ErrorResponse "Erro interno ao listar histórico"
// @Router /api/v1/matches [get]
func (h *ProcessorHandler) ListMatches(w http.ResponseWriter, r *http.Request) {
	records, err := h.orchestrator.ListMatches(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list matches: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, records)
}

// GetMatchRecord consulta um item do histórico de matches pelo parâmetro de
// rota {id} via h.orchestrator. Responde 200 com o domain.MatchRecord, ou
// 400/404 com a mensagem de erro.
// @Summary Consulta um item do histórico de matches
// @Description Retorna um item específico do histórico (em memória) de execuções de match pelo seu ID.
// @Tags Processador
// @Produce json
// @Param id path string true "ID do match"
// @Success 200 {object} domain.MatchRecord "Item do histórico"
// @Failure 404 {object} domain.ErrorResponse "Match não encontrado"
// @Router /api/v1/matches/{id} [get]
func (h *ProcessorHandler) GetMatchRecord(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondWithError(w, http.StatusBadRequest, "Match ID is required")
		return
	}

	record, err := h.orchestrator.GetMatch(r.Context(), id)
	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, record)
}

// DeleteMatch remove um item do histórico de matches pelo parâmetro de rota
// {id} via h.orchestrator. Responde 200 com status "success", ou 400/404 com a
// mensagem de erro.
// @Summary Remove um item do histórico de matches
// @Description Remove um item específico do histórico (em memória) de execuções de match pelo seu ID.
// @Tags Processador
// @Produce json
// @Param id path string true "ID do match"
// @Success 200 {object} map[string]string "Match removido com sucesso"
// @Failure 404 {object} domain.ErrorResponse "Match não encontrado"
// @Router /api/v1/matches/{id} [delete]
func (h *ProcessorHandler) DeleteMatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondWithError(w, http.StatusBadRequest, "Match ID is required")
		return
	}

	if err := h.orchestrator.DeleteMatch(r.Context(), id); err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Match deleted successfully",
	})
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message, "status": "error"})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
