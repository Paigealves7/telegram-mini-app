package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type SawingHandler struct {
	repo *repository.Repository
}

func NewSawingHandler(repo *repository.Repository) *SawingHandler {
	return &SawingHandler{repo: repo}
}

type CreateSawingRequest struct {
	WorkerID int64             `json:"worker_id"`
	Date     string            `json:"date"`
	Logs     []domain.SawedLog `json:"logs"`
	Boards   []domain.Board    `json:"boards"`
}

func (h *SawingHandler) CreateSawing(w http.ResponseWriter, r *http.Request) {
	var req CreateSawingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Logs) == 0 || len(req.Boards) == 0 {
		http.Error(w, "некорректный JSON или пустой список бревен/досок", http.StatusBadRequest)
		return
	}

	operation := &domain.SawingOperation{
		WorkerID: req.WorkerID,
		Date:     req.Date,
	}

	result, err := h.repo.CreateSawingOperation(r.Context(), operation, req.Logs, req.Boards)
	if err != nil {
		http.Error(w, "ошибка сохранения распила: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}
