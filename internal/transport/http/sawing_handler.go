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
	Date   string            `json:"date" validate:"required"`
	Logs   []domain.SawedLog `json:"logs" validate:"required,min=1,dive"`
	Boards []domain.Board    `json:"boards" validate:"required,min=1,dive"`
}

func (h *SawingHandler) CreateSawing(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}

	var req CreateSawingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}

	if err := Validate.Struct(req); err != nil {
		http.Error(w, validationMessage(err), http.StatusBadRequest)
		return
	}

	if _, err := parseDate(req.Date); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	operation := &domain.SawingOperation{
		WorkerID: user.UserID, // из JWT, а не из тела запроса
		Date:     req.Date,
	}

	result, err := h.repo.CreateSawingOperation(r.Context(), operation, req.Logs, req.Boards)
	if err != nil {
		respondError(w, r, "не удалось сохранить распил", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}
