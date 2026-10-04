package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type BoardHandler struct {
	repo *repository.Repository
}

func NewBoardHandler(repo *repository.Repository) *BoardHandler {
	return &BoardHandler{repo: repo}
}

func (h *BoardHandler) GetList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0

	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	filter := domain.BoardFilter{
		Species: r.URL.Query().Get("species"),
		Grade:   r.URL.Query().Get("grade"),
	}
	if val, err := strconv.Atoi(r.URL.Query().Get("height_mm")); err == nil {
		filter.HeightMM = val
	}
	if val, err := strconv.Atoi(r.URL.Query().Get("width_mm")); err == nil {
		filter.WidthMM = val
	}
	if val, err := strconv.Atoi(r.URL.Query().Get("length_mm")); err == nil {
		filter.LengthMM = val
	}

	boards, err := h.repo.GetAllBoards(r.Context(), filter, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"не удалось получить список досок"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(boards)
}
