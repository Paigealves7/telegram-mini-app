package http

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"math"
	"net/http"
	"strconv"
	"strings"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
	"time"
)

func (h *FirewoodHandler) CreateProcessing(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", 401)
		return
	}
	var p domain.FirewoodProcessing
	if json.NewDecoder(r.Body).Decode(&p) != nil {
		http.Error(w, "некорректный JSON", 400)
		return
	}
	p.Performer = strings.TrimSpace(p.Performer)
	p.Comment = strings.TrimSpace(p.Comment)
	if (p.Kind != "split" && p.Kind != "round") || !validFirewoodVolume(p.VolumeM3) || len([]rune(p.Performer)) < 1 || len([]rune(p.Performer)) > 120 || len([]rune(p.Comment)) > 1000 {
		http.Error(w, "проверьте вид, объём, исполнителя и комментарий", 400)
		return
	}
	if _, err := time.Parse("2006-01-02", p.Date); err != nil {
		http.Error(w, "дата: формат ГГГГ-ММ-ДД", 400)
		return
	}
	if err := h.repo.CreateFirewoodProcessing(r.Context(), &p, user.UserID); err != nil {
		respondError(w, r, "не удалось записать переработку", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(p)
}
func validFirewoodVolume(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0.001 && v <= 100000 && math.Abs(v*1000-math.Round(v*1000)) < 1e-7
}
func (h *FirewoodHandler) GetProcessing(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r, 100, 1000)
	list, err := h.repo.FirewoodProcessing(r.Context(), limit, offset)
	if err != nil {
		serverError(w, r, "не удалось получить переработку", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}
func (h *FirewoodHandler) DeleteProcessing(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "неверный ID", 400)
		return
	}
	err = h.repo.DeleteFirewoodProcessing(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "операция не найдена", 404)
		return
	}
	if errors.Is(err, repository.ErrInsufficientStock) {
		http.Error(w, "готовые дрова уже отгружены", 409)
		return
	}
	if err != nil {
		serverError(w, r, "не удалось удалить переработку", err)
		return
	}
	w.WriteHeader(204)
}
func (h *FirewoodHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.FirewoodSettings(r.Context())
	if err != nil {
		serverError(w, r, "не удалось загрузить коэффициенты", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}
func (h *FirewoodHandler) SetSettings(w http.ResponseWriter, r *http.Request) {
	var s struct {
		Split float64 `json:"split"`
		Round float64 `json:"round"`
	}
	if json.NewDecoder(r.Body).Decode(&s) != nil || !(s.Split >= 0.0001 && s.Split <= 1 && s.Round >= 0.0001 && s.Round <= 1) || math.Abs(s.Split*10000-math.Round(s.Split*10000)) > 1e-7 || math.Abs(s.Round*10000-math.Round(s.Round*10000)) > 1e-7 {
		http.Error(w, "коэффициенты: от 0.0001 до 1, до четырёх знаков после запятой", 400)
		return
	}
	if err := h.repo.UpdateFirewoodSettings(r.Context(), s.Split, s.Round); err != nil {
		serverError(w, r, "не удалось сохранить коэффициенты", err)
		return
	}
	h.GetSettings(w, r)
}
