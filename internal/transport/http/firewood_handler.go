package http

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"telegram-mini-app/internal/repository"

	"github.com/jackc/pgx/v5/pgconn"
)

// FirewoodHandler: дрова (берёза), учёт общей кубатурой.
type FirewoodHandler struct {
	repo *repository.Repository
}

func NewFirewoodHandler(repo *repository.Repository) *FirewoodHandler {
	return &FirewoodHandler{repo: repo}
}

func (h *FirewoodHandler) CreateArrival(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<12)
	var req struct {
		SupplierID  int64   `json:"supplier_id"`
		CarrierID   int64   `json:"carrier_id"`
		TruckNumber string  `json:"truck_number"`
		VolumeM3    float64 `json:"volume_m3"`
		ArrivalDate string  `json:"arrival_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	if req.SupplierID <= 0 || req.CarrierID <= 0 {
		http.Error(w, "выберите поставщика и перевозчика", http.StatusBadRequest)
		return
	}
	truck := strings.TrimSpace(req.TruckNumber)
	if n := len([]rune(truck)); n < 1 || n > 30 {
		http.Error(w, "номер машины: от 1 до 30 символов", http.StatusBadRequest)
		return
	}
	if math.IsNaN(req.VolumeM3) || math.IsInf(req.VolumeM3, 0) || req.VolumeM3 <= 0 || req.VolumeM3 > 100000 {
		http.Error(w, "объём должен быть больше нуля", http.StatusBadRequest)
		return
	}
	date := strings.TrimSpace(req.ArrivalDate)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "дата: формат ГГГГ-ММ-ДД", http.StatusBadRequest)
		return
	}

	a, err := h.repo.CreateFirewoodArrival(r.Context(), req.SupplierID, req.CarrierID, truck, math.Round(req.VolumeM3*1000)/1000, date, user.UserID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		http.Error(w, "поставщик или перевозчик не найден", http.StatusBadRequest)
		return
	}
	if err != nil {
		serverError(w, r, "не удалось записать приход дров", err)
		return
	}
	log.Printf("FIREWOOD: пользователь %d записал приход дров %.3f м³ (№%d)", user.UserID, a.VolumeM3, a.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(a)
}

func (h *FirewoodHandler) GetArrivals(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r, 100, 1000)
	list, err := h.repo.GetFirewoodArrivals(r.Context(), limit, offset)
	if err != nil {
		serverError(w, r, "не удалось получить приходы дров", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (h *FirewoodHandler) GetStock(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.GetFirewoodStock(r.Context())
	if err != nil {
		serverError(w, r, "не удалось получить склад дров", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}
