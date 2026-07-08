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

// NewProcessorHandler creates a new handler instance.
func NewProcessorHandler(orchestrator domain.Orchestrator) *ProcessorHandler {
	return &ProcessorHandler{
		orchestrator: orchestrator,
	}
}


// Clear handles POST /api/v1/vacancies/clear
// @Summary Limpa o banco de dados vetorial de vagas
// @Description Remove todas as vagas e coleções existentes no banco de dados local do chromem-go.
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

type populateRequest struct {
	Content   string `json:"content"`
	Delimiter string `json:"delimiter"`
}

// Populate handles POST /api/v1/vacancies
// @Summary Popula o banco vetorial de vagas (Assíncrono)
// @Description Recebe uma lista de vagas em texto bruto e um delimitador. Inicia a divisão do texto, gera os embeddings vetoriais via Ollama em background e as salva no banco vetorial local (chromem-go). Retorna imediatamente com o task_id.
// @Tags Vagas
// @Accept json
// @Produce json
// @Param request body populateRequest true "Payload com as vagas e delimitador"
// @Success 202 {object} map[string]interface{} "Tarefa de população iniciada com sucesso"
// @Failure 400 {object} domain.ErrorResponse "Requisição inválida"
// @Failure 500 {object} domain.ErrorResponse "Erro ao processar e salvar vagas"
// @Router /api/v1/vacancies [post]
func (h *ProcessorHandler) Populate(w http.ResponseWriter, r *http.Request) {
	var req populateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if req.Content == "" {
		respondWithError(w, http.StatusBadRequest, "Field 'content' is required")
		return
	}
	if req.Delimiter == "" {
		respondWithError(w, http.StatusBadRequest, "Field 'delimiter' is required")
		return
	}

	taskID, err := h.orchestrator.PopulateVacanciesAsync(r.Context(), req.Content, req.Delimiter)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to start database population: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":  "accepted",
		"task_id": taskID,
		"message": "Vacancies processing started in background",
	})
}

// GetTaskStatus handles GET /api/v1/tasks/{id}
// @Summary Consulta o status de uma tarefa em background
// @Description Retorna o estado atual de processamento de uma tarefa assíncrona (como a geração de embeddings de vagas).
// @Tags Tarefas
// @Produce json
// @Param id path string true "ID da tarefa"
// @Success 200 {object} domain.TaskStatus "Informações e status da tarefa"
// @Failure 404 {object} domain.ErrorResponse "Tarefa não encontrada"
// @Router /api/v1/tasks/{id} [get]
func (h *ProcessorHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondWithError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	task, err := h.orchestrator.GetTaskStatus(r.Context(), id)
	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, task)
}

type matchRequest struct {
	FileBase64     string `json:"file_base64"`
	CandidateEmail string `json:"candidate_email,omitempty"`
	CandidatePhone string `json:"candidate_phone,omitempty"`
}

// Match handles POST /api/v1/match
// @Summary Executa match de currículo contra banco de vagas
// @Description Recebe apenas o currículo do candidato em base64 e identifica o candidato para disparos dinâmicos.
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

	// Clean base64 prefix if present
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
