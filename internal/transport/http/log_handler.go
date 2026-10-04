package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type LogHandler struct {
	repo *repository.Repository
}

func NewLogHandler(repo *repository.Repository) *LogHandler {
	return &LogHandler{repo: repo}
}

func (h *LogHandler) GetList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0

	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	filter := domain.LogFilter{
		Species: r.URL.Query().Get("species"),
	}
	if val, err := strconv.Atoi(r.URL.Query().Get("length_mm")); err == nil {
		filter.LengthMM = val
	}
	if val, err := strconv.Atoi(r.URL.Query().Get("diameter_mm")); err == nil {
		filter.DiameterMM = val
	}

	logs, err := h.repo.GetAllLogs(r.Context(), filter, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"не удалось получить список сырья"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
