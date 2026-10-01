package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/repository"
)

type StatsHandler struct {
	repo *repository.Repository
}

func NewStatsHandler(repo *repository.Repository) *StatsHandler {
	return &StatsHandler{repo: repo}
}

// GET /api/v1/statistics?start_date=2026-09-01&end_date=2026-09-30
func (h *StatsHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if startDate == "" || endDate == "" {
		http.Error(w, "Параметры start_date и end_date обязательны", http.StatusBadRequest)
		return
	}

	stats, err := h.repo.GetStatisticsSummary(r.Context(), startDate, endDate)
	if err != nil {
		http.Error(w, "Ошибка получения статистики: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
