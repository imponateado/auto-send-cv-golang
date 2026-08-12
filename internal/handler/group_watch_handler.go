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

// ListGroups gerencia GET /api/v1/whatsapp/groups?phone=...
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

// GetWatchedGroups gerencia GET /api/v1/whatsapp/groups/watched?phone=...
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

// SetWatchedGroups gerencia PUT /api/v1/whatsapp/groups/watched?phone=...
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

// GetSchedule gerencia GET /api/v1/whatsapp/groups/schedule
func (h *GroupWatchHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	sched, err := h.svc.GetSchedule(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get schedule: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, sched)
}

// SetSchedule gerencia PUT /api/v1/whatsapp/groups/schedule
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

// FlushNow gerencia POST /api/v1/whatsapp/groups/flush
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

// Status gerencia GET /api/v1/whatsapp/groups/status
func (h *GroupWatchHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.Status(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get status: "+err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, status)
}
