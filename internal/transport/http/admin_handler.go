package http

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type AdminHandler struct {
	repo *repository.Repository
}

func (h AdminHandler) ResetTestData(w http.ResponseWriter, r *http.Request) {
	if orgID, ok := requireOrgID(w, r); ok {
		OrganizationHandler{repo: h.repo}.resetOrganization(w, r, orgID)
	}
}

func adminID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "некорректный номер записи", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func adminResult(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, repository.ErrOrganizationNotFound):
		http.Error(w, "запись не найдена", http.StatusNotFound)
	case errors.Is(err, repository.ErrOperationInUse), errors.Is(err, repository.ErrOrderSaleLink), errors.Is(err, repository.ErrResetNotDemo):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		respondError(w, r, what, err)
	}
}

func deleteOperation(remove func(context.Context, int64, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := requireOrgID(w, r)
		if !ok {
			return
		}
		if id, ok := adminID(w, r); ok {
			adminResult(w, r, "не удалось удалить запись", remove(r.Context(), orgID, id))
		}
	}
}

func validStockNumber(n, max, scale float64) bool {
	return n >= 0 && n < max && math.Abs(n*scale-math.Round(n*scale)) < 1e-4
}

func (h AdminHandler) UpdateBoard(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return
	}
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	var req struct {
		domain.Board
		Count    *int     `json:"count"`
		VolumeM3 *float64 `json:"volume_m3"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	b := req.Board
	b.ID, b.Species, b.Grade = id, strings.TrimSpace(b.Species), strings.TrimSpace(b.Grade)
	if b.Species == "" || len([]rune(b.Species)) > 64 || len([]rune(b.Grade)) > 64 ||
		b.HeightMM <= 0 || b.WidthMM <= 0 || b.LengthMM <= 0 ||
		b.HeightMM > math.MaxInt32 || b.WidthMM > math.MaxInt32 || b.LengthMM > math.MaxInt32 {
		http.Error(w, "укажите породу, сорт и корректные размеры доски", http.StatusBadRequest)
		return
	}
	if req.Count == nil || req.VolumeM3 == nil || *req.Count < 0 || *req.Count > math.MaxInt32 ||
		!validStockNumber(*req.VolumeM3, 1e9, 1000) || (*req.Count == 0) != (*req.VolumeM3 == 0) {
		http.Error(w, "укажите количество и объём досок, до трёх знаков после запятой", http.StatusBadRequest)
		return
	}
	if b.PricePerM3 != nil && !validStockNumber(*b.PricePerM3, 1e10, 100) {
		http.Error(w, "укажите неотрицательную цену, до двух знаков после запятой", http.StatusBadRequest)
		return
	}
	b.Count, b.VolumeM3 = *req.Count, *req.VolumeM3
	actor, _ := currentUser(r)
	adminResult(w, r, "не удалось сохранить позицию склада", h.repo.UpdateBoard(r.Context(), orgID, b, actor.UserID))
}

func (h AdminHandler) SetFirewoodStock(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return
	}
	var req struct {
		Raw   *float64 `json:"raw_m3"`
		Split *float64 `json:"split_m3"`
		Round *float64 `json:"round_m3"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	if req.Raw == nil || req.Split == nil || req.Round == nil ||
		!validStockNumber(*req.Raw, 1e9, 1e7) ||
		!validStockNumber(*req.Split, 1e9, 1000) || !validStockNumber(*req.Round, 1e9, 1000) {
		http.Error(w, "укажите неотрицательные остатки: сырьё до семи, готовые дрова до трёх знаков после запятой", http.StatusBadRequest)
		return
	}
	actor, _ := currentUser(r)
	stock := domain.FirewoodStock{RawM3: *req.Raw, SplitM3: *req.Split, RoundM3: *req.Round}
	adminResult(w, r, "не удалось скорректировать остатки дров", h.repo.SetFirewoodStock(r.Context(), orgID, stock, actor.UserID))
}
