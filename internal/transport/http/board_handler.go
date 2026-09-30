package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/repository"
)

type BoardHandler struct {
	repo *repository.Repository
}

func NewBoardHandler(repo *repository.Repository) *BoardHandler {
	return &BoardHandler{repo: repo}
}

func (h *BoardHandler) GetList(w http.ResponseWriter, r *http.Request) {
	boards, err := h.repo.GetAllBoards(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to fetch boards"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(boards)
}
