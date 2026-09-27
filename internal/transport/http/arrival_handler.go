package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type ArrivalHandler struct {
	repo *repository.Repository
}

func NewArrivalHandler(repo *repository.Repository) *ArrivalHandler {
	return &ArrivalHandler{repo: repo}
}

type CreateArrivalRequest struct {
	SupplierID  int64            `json:"supplier_id"`
	CarrierID   int64            `json:"carrier_id"`
	TruckNumber string           `json:"truck_number"`
	ArrivalDate string           `json:"arrival_date"`
	CreatedBy   int64            `json:"created_by"`
	Items       []domain.LogItem `json:"items"`
}

// POST /api/v1/arrivals
func (h *ArrivalHandler) CreateArrival(w http.ResponseWriter, r *http.Request) {
	var req CreateArrivalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		http.Error(w, "некорректный JSON или пустой список бревен", http.StatusBadRequest)
		return
	}

	arrival := &domain.LogArrival{
		SupplierID:  req.SupplierID,
		CarrierID:   req.CarrierID,
		TruckNumber: req.TruckNumber,
		ArrivalDate: req.ArrivalDate,
		CreatedBy:   req.CreatedBy,
	}

	result, err := h.repo.CreateLogArrival(r.Context(), arrival, req.Items)
	if err != nil {
		http.Error(w, "ошибка сохранения прихода: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}
