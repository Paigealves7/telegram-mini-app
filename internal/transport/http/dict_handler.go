package http

import (
	"encoding/json"
	"net/http"
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contractors)
}

func (h *DictHandler) CreateContractor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	contractor, err := h.repo.CreateContractor(r.Context(), req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(contractor)
}
