package handler

import (
	"encoding/json"
	"net/http"

	"api/internal/domain"
	"api/internal/service"
)

type GroupWatchHandler struct {
	svc *service.GroupWatcher
}

func NewGroupWatchHandler(svc *service.GroupWatcher) *GroupWatchHandler {
	return &GroupWatchHandler{svc: svc}
}

// ListGroups lista os grupos do WhatsApp de que o número em ?phone= é membro.
// Responde 200 com a lista de domain.GroupInfo, ou 400/500 com a mensagem de erro.
func (h *GroupWatchHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respondWithError(w, http.StatusBadRequest, "Query parameter 'phone' is required")
		return
	}

	groups, err := h.svc.ListJoinedGroups(r.Context(), phone)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list groups: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, groups)
}

// GetWatchedGroups lista os grupos atualmente observados para o número em
// ?phone=. Responde 200 com a lista de domain.WatchedGroup, ou 400/500 com a
// mensagem de erro.
func (h *GroupWatchHandler) GetWatchedGroups(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respondWithError(w, http.StatusBadRequest, "Query parameter 'phone' is required")
		return
	}

	groups, err := h.svc.GetWatchedGroups(r.Context(), phone)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get watched groups: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, groups)
}

type setWatchedGroupsRequest struct {
	Groups []domain.GroupInfo `json:"groups"`
}

// SetWatchedGroups substitui o conjunto de grupos observados para o número em
// ?phone= pelo corpo da requisição. Responde 200 com status "success", ou
// 400/500 com a mensagem de erro.
func (h *GroupWatchHandler) SetWatchedGroups(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		respondWithError(w, http.StatusBadRequest, "Query parameter 'phone' is required")
		return
	}

	var req setWatchedGroupsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if err := h.svc.SetWatchedGroups(r.Context(), phone, req.Groups); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to set watched groups: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Watched groups updated successfully",
	})
}

// GetSchedule consulta o horário diário global de flush do buffer. Responde 200
// com o domain.FlushSchedule, ou 500 com a mensagem de erro.
func (h *GroupWatchHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	sched, err := h.svc.GetSchedule(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get schedule: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, sched)
}

// SetSchedule valida e atualiza o horário diário global de flush do buffer.
// Responde 200 com status "success", ou 400/500 com a mensagem de erro.
func (h *GroupWatchHandler) SetSchedule(w http.ResponseWriter, r *http.Request) {
	var req domain.FlushSchedule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}
	if req.Hour < 0 || req.Hour > 23 || req.Minute < 0 || req.Minute > 59 {
		respondWithError(w, http.StatusBadRequest, "Fields 'hour' (0-23) and 'minute' (0-59) must be valid")
		return
	}

	if err := h.svc.SetSchedule(r.Context(), req); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to set schedule: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Schedule updated successfully",
	})
}

// FlushNow dispara imediatamente o processamento do buffer de mensagens
// pendentes. Responde 200 com a quantidade de itens processados, ou 500 com a
// mensagem de erro.
func (h *GroupWatchHandler) FlushNow(w http.ResponseWriter, r *http.Request) {
	count, err := h.svc.FlushNow(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to flush buffer: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "success",
		"items_processed": count,
	})
}

// Status consulta a contagem de mensagens pendentes e o resultado da última
// rodada de flush. Responde 200 com o service.FlushStatus, ou 500 com a mensagem
// de erro.
func (h *GroupWatchHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.Status(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get status: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, status)
}
