package http

import (
	"encoding/json"
	"net/http"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/service"
)

type SalesHandler struct {
	service *service.SalesService
}

func NewSalesHandler(service *service.SalesService) *SalesHandler {
	return &SalesHandler{service: service}
}

type CreateSaleRequest struct {
	ContractorID  int64             `json:"contractor_id" validate:"required,gt=0"`
	TruckNumber   string            `json:"truck_number" validate:"required"`
	DeliveryPrice float64           `json:"delivery_price" validate:"gte=0"`
	ExtraPrice    float64           `json:"extra_price" validate:"gte=0"`
	Items         []domain.SaleItem `json:"items" validate:"required,min=1,dive"`
}

func (h *SalesHandler) CreateSale(w http.ResponseWriter, r *http.Request) {
	var req CreateSaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}

	if err := Validate.Struct(req); err != nil {
		http.Error(w, "ошибка валидации данных: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Извлекаем ID пользователя из контекста (токен)
	claims, ok := r.Context().Value(UserContextKey).(*Claims)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}

	sale := &domain.Sale{
		ContractorID:  req.ContractorID,
		TruckNumber:   req.TruckNumber,
		DeliveryPrice: req.DeliveryPrice,
		ExtraPrice:    req.ExtraPrice,
		CreatedBy:     claims.UserID,
	}

	result, err := h.service.CreateSale(r.Context(), sale, req.Items)
	if err != nil {
		http.Error(w, "ошибка проведения продажи: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}
