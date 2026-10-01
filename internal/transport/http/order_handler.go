package http

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	MasterID      int64              `json:"master_id" validate:"required,gt=0"`
	ContractorID  int64              `json:"contractor_id" validate:"required,gt=0"`
	Notes         string             `json:"notes"`
	DeliveryPrice float64            `json:"delivery_price" validate:"gte=0"`
	ExtraPrice    float64            `json:"extra_price" validate:"gte=0"`
	Items         []domain.OrderItem `json:"items" validate:"required,min=1,dive"`
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}

	if err := Validate.Struct(req); err != nil {
		http.Error(w, "ошибка валидации данных: "+err.Error(), http.StatusBadRequest)
		return
	}

	order := &domain.Order{
		MasterID:      req.MasterID,
		ContractorID:  req.ContractorID,
		Notes:         req.Notes,
		DeliveryPrice: req.DeliveryPrice,
		ExtraPrice:    req.ExtraPrice,
		Status:        domain.OrderStatusNew,
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

func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0

	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	orders, err := h.repo.GetOrders(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "ошибка получения заказов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}
