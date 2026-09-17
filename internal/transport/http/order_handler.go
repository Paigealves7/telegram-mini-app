package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type OrderHandler struct {
	repo *repository.Repository
}

func NewOrderHandler(repo *repository.Repository) *OrderHandler {
	return &OrderHandler{repo: repo}
}

type CreateOrderRequest struct {
	CustomerID  int64              `json:"customer_id"`
	TotalAmount float64            `json:"total_amount"`
	Items       []domain.OrderItem `json:"items"`
}

// CreateOrder — POST /api/v1/orders (Раздел 4 ТЗ)
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		http.Error(w, "некорректный JSON или пустой список позиций", http.StatusBadRequest)
		return
	}

	order := &domain.Order{
		CustomerID:  req.CustomerID,
		Status:      domain.OrderStatusPending,
		TotalAmount: req.TotalAmount,
	}

	result, err := h.repo.CreateOrder(r.Context(), order, req.Items)
	if err != nil {
		http.Error(w, "ошибка создания заказа: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// GetOrders — GET /api/v1/orders (Раздел 4 ТЗ)
func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.repo.GetOrders(r.Context())
	if err != nil {
		http.Error(w, "ошибка получения заказов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}
