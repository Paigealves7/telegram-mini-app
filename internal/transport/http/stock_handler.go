package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"telegram-mini-app/internal/repository"
)

// StockHandler: корректировки склада (списание, излишки) и их журнал. Только для менеджера (см. router.go).
type StockHandler struct {
	repo *repository.Repository
}

func NewStockHandler(repo *repository.Repository) *StockHandler {
	return &StockHandler{repo: repo}
}

var adjustReasons = map[string]bool{"defect": true, "loss": true, "own_use": true, "surplus": true, "other": true}

func (h *StockHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	var req struct {
		Kind   string `json:"kind"`
		ItemID int64  `json:"item_id"`
		Count  int    `json:"count"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	if req.Kind != "board" && req.Kind != "log" {
		http.Error(w, "укажите, что корректируем: пиломатериал или сырьё", http.StatusBadRequest)
		return
	}
	if req.ItemID <= 0 {
		http.Error(w, "не выбрана позиция склада", http.StatusBadRequest)
		return
	}
	if req.Count <= 0 || req.Count > 1000000 {
		http.Error(w, "количество должно быть больше нуля", http.StatusBadRequest)
		return
	}
	if !adjustReasons[req.Reason] {
		http.Error(w, "выберите причину", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 200 {
		http.Error(w, "пояснение: не больше 200 символов", http.StatusBadRequest)
		return
	}
	if req.Reason == "other" && note == "" {
		http.Error(w, "для причины «Другое» напишите пояснение", http.StatusBadRequest)
		return
	}

	adj, err := h.repo.AdjustStock(r.Context(), req.Kind, req.ItemID, req.Count, req.Reason, note, user.UserID)
	switch {
	case errors.Is(err, repository.ErrBoardNotFound), errors.Is(err, repository.ErrLogNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case errors.Is(err, repository.ErrInsufficientStock), errors.Is(err, repository.ErrLogSurplus):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		serverError(w, r, "не удалось записать корректировку", err)
		return
	}
	log.Printf("STOCK: пользователь %d: %s #%d, %+d шт, причина %s", user.UserID, adj.Kind, adj.ItemID, adj.DeltaCount, adj.Reason)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(adj)
}

func (h *StockHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r, 200, 1000)
	list, err := h.repo.ListStockAdjustments(r.Context(), limit, offset)
	if err != nil {
		serverError(w, r, "не удалось получить журнал корректировок", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}
