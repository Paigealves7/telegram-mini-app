package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type SalesHandler struct {
	repo *repository.Repository
}

func NewSalesHandler(repo *repository.Repository) *SalesHandler {
	return &SalesHandler{repo: repo}
}

type CreateSaleRequest struct {
	BuyerID     int64             `json:"buyer_id"`
	TotalAmount float64           `json:"total_amount"`
	SaleDate    string            `json:"sale_date"`
	CreatedBy   int64             `json:"created_by"`
	Items       []domain.SaleItem `json:"items"`
}

// CreateSale — POST /api/v1/sales (Раздел 4 ТЗ)
func (h *SalesHandler) CreateSale(w http.ResponseWriter, r *http.Request) {
	var req CreateSaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		http.Error(w, "некорректный JSON или пустой список позиций", http.StatusBadRequest)
		return
	}

	sale := &domain.Sale{
		BuyerID:     req.BuyerID,
		TotalAmount: req.TotalAmount,
		SaleDate:    req.SaleDate,
		CreatedBy:   req.CreatedBy,
	}

	result, err := h.repo.CreateSale(r.Context(), sale, req.Items)
	if err != nil {
		http.Error(w, "ошибка проведения продажи: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}
