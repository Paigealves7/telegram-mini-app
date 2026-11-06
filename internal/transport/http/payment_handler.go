package http

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi/v5"
)

// PaymentHandler: оплаты по заказам и долги клиентов.
type PaymentHandler struct {
	repo *repository.Repository
}

func NewPaymentHandler(repo *repository.Repository) *PaymentHandler {
	return &PaymentHandler{repo: repo}
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "некорректный номер", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (h *PaymentHandler) List(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	res, err := h.repo.GetOrderPayments(r.Context(), id)
	if errors.Is(err, repository.ErrOrderNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, r, "не удалось получить оплаты", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (h *PaymentHandler) Add(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	var req struct {
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount <= 0 || req.Amount > 1e9 {
		http.Error(w, "сумма оплаты должна быть больше нуля", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 200 {
		http.Error(w, "заметка: не больше 200 символов", http.StatusBadRequest)
		return
	}

	p, err := h.repo.AddPayment(r.Context(), id, req.Amount, note, user.UserID)
	switch {
	case errors.Is(err, repository.ErrOrderNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case errors.Is(err, repository.ErrPaymentTooLarge), errors.Is(err, repository.ErrOrderCanceledPay):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		serverError(w, r, "не удалось записать оплату", err)
		return
	}
	log.Printf("PAYMENT: пользователь %d принял оплату %.2f по заказу %d", user.UserID, p.Amount, id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

// Delete: убрать ошибочную оплату (только менеджер, см. router.go).
func (h *PaymentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	deleted, err := h.repo.DeletePayment(r.Context(), id)
	if err != nil {
		serverError(w, r, "не удалось удалить оплату", err)
		return
	}
	if !deleted {
		http.Error(w, "оплата не найдена", http.StatusNotFound)
		return
	}
	if user, ok := currentUser(r); ok {
		log.Printf("PAYMENT: пользователь %d удалил оплату %d", user.UserID, id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PaymentHandler) Debts(w http.ResponseWriter, r *http.Request) {
	debts, err := h.repo.GetDebts(r.Context())
	if err != nil {
		serverError(w, r, "не удалось получить долги", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(debts)
}
