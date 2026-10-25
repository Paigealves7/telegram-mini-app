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
	SupplierID  int64            `json:"supplier_id" validate:"required,gt=0"`
	CarrierID   int64            `json:"carrier_id" validate:"required,gt=0"`
	TruckNumber string           `json:"truck_number" validate:"required,max=20"`
	ArrivalDate string           `json:"arrival_date" validate:"required"`
	Items       []domain.LogItem `json:"items" validate:"required,min=1,dive"` // dive валидирует элементы массива!
}

func (h *ArrivalHandler) CreateArrival(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}

	var req CreateArrivalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}

	if err := Validate.Struct(req); err != nil {
		http.Error(w, validationMessage(err), http.StatusBadRequest)
		return
	}

	if _, err := parseDate(req.ArrivalDate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	arrival := &domain.LogArrival{
		SupplierID:  req.SupplierID,
		CarrierID:   req.CarrierID,
		TruckNumber: req.TruckNumber,
		ArrivalDate: req.ArrivalDate,
		CreatedBy:   user.UserID, // из JWT, а не из тела запроса
	}

	result, err := h.repo.CreateLogArrival(r.Context(), arrival, req.Items)
	if err != nil {
		respondError(w, r, "не удалось сохранить приход", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}
