package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi/v5"
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
		serverError(w, r, "не удалось получить список контрагентов", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contractors)
}

type contractorReq struct {
	Name       string `json:"name"`
	IsSupplier bool   `json:"is_supplier"`
	IsCarrier  bool   `json:"is_carrier"`
	IsClient   bool   `json:"is_client"`
	IsActive   bool   `json:"is_active"`
}

// parseContractor читает и проверяет тело запроса; при ошибке сам отвечает клиенту.
func (h *DictHandler) parseContractor(w http.ResponseWriter, r *http.Request, selfID int64) (*contractorReq, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	var req contractorReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return nil, false
	}
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	if n := len([]rune(req.Name)); n < 2 || n > 100 {
		http.Error(w, "название: от 2 до 100 символов", http.StatusBadRequest)
		return nil, false
	}
	if !req.IsSupplier && !req.IsCarrier && !req.IsClient {
		http.Error(w, "выберите хотя бы один тип: поставщик, перевозчик или клиент", http.StatusBadRequest)
		return nil, false
	}
	// Дубли без учёта регистра и лишних пробелов.
	all, err := h.repo.GetContractors(r.Context())
	if err != nil {
		serverError(w, r, "не удалось проверить список контрагентов", err)
		return nil, false
	}
	for _, c := range all {
		if c.ID != selfID && strings.EqualFold(strings.Join(strings.Fields(c.Name), " "), req.Name) {
			http.Error(w, "такой контрагент уже есть: «"+c.Name+"»", http.StatusConflict)
			return nil, false
		}
	}
	return &req, true
}

func (h *DictHandler) CreateContractor(w http.ResponseWriter, r *http.Request) {
	req, ok := h.parseContractor(w, r, 0)
	if !ok {
		return
	}
	contractor, err := h.repo.CreateContractorTyped(r.Context(), req.Name, req.IsSupplier, req.IsCarrier, req.IsClient)
	if err != nil {
		respondError(w, r, "не удалось добавить контрагента", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(contractor)
}

func contractorID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "некорректный номер контрагента", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (h *DictHandler) UpdateContractor(w http.ResponseWriter, r *http.Request) {
	id, ok := contractorID(w, r)
	if !ok {
		return
	}
	req, ok := h.parseContractor(w, r, id)
	if !ok {
		return
	}
	updated, err := h.repo.UpdateContractor(r.Context(), id, req.Name, req.IsSupplier, req.IsCarrier, req.IsClient, req.IsActive)
	if err != nil {
		respondError(w, r, "не удалось сохранить контрагента", err)
		return
	}
	if !updated {
		http.Error(w, "контрагент не найден", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DictHandler) DeleteContractor(w http.ResponseWriter, r *http.Request) {
	id, ok := contractorID(w, r)
	if !ok {
		return
	}
	deleted, err := h.repo.DeleteContractor(r.Context(), id)
	if errors.Is(err, repository.ErrContractorInUse) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		serverError(w, r, "не удалось удалить контрагента", err)
		return
	}
	if !deleted {
		http.Error(w, "контрагент не найден", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
