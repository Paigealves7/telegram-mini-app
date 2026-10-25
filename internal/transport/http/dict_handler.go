package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"telegram-mini-app/internal/repository"
)

type DictHandler struct {
	repo *repository.Repository
}

func NewDictHandler(repo *repository.Repository) *DictHandler {
	return &DictHandler{repo: repo}
}

func (h *DictHandler) GetContractors(w http.ResponseWriter, r *http.Request) {
	contractors, err := h.repo.GetContractors(r.Context())
	if err != nil {
		serverError(w, r, "не удалось получить список клиентов", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contractors)
}

func (h *DictHandler) CreateContractor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 100 {
		http.Error(w, "название клиента: от 1 до 100 символов", http.StatusBadRequest)
		return
	}

	contractor, err := h.repo.CreateContractor(r.Context(), name)
	if err != nil {
		respondError(w, r, "не удалось добавить клиента", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(contractor)
}
