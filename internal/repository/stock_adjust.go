package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
)

var ErrLogSurplus = errors.New("излишек можно добавить только для пиломатериалов")

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func lenLabel(mm int) string {
	return strings.Replace(strconv.FormatFloat(float64(mm)/1000, 'f', -1, 64), ".", ",", 1) + " м"
}

// AdjustStock списывает (брак, потеря, свои нужды, другое) или добавляет (излишек) count штук позиции склада
// и записывает операцию в журнал. Объём пересчитывается пропорционально остатку, как при продаже.
func (r *Repository) AdjustStock(ctx context.Context, kind string, itemID int64, count int, reason, note string, userID int64) (*domain.StockAdjustment, error) {
	sign := -1
	if reason == "surplus" {
		sign = 1
	}
	if kind == "log" && sign > 0 {
		return nil, ErrLogSurplus
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	adj := domain.StockAdjustment{Kind: kind, ItemID: itemID, Reason: reason, Note: note, CreatedBy: userID, DeltaCount: sign * count}

	switch kind {
	case "board":
		var h, w, l, cnt int
		var species, grade string
		var vol float64
		err := tx.QueryRow(ctx, `SELECT height_mm, width_mm, length_mm, species, grade, count, volume_m3 FROM boards WHERE id = $1 FOR UPDATE`, itemID).
			Scan(&h, &w, &l, &species, &grade, &cnt, &vol)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrBoardNotFound
			}
			return nil, err
		}
		var dv float64
		if sign < 0 {
			if count > cnt {
				return nil, ErrInsufficientStock
			}
			dv = domain.ItemVolume(cnt, vol, count)
		} else {
			dv = round3(float64(h) * float64(w) * float64(l) / 1e9 * float64(count))
		}
		newCnt := cnt + sign*count
		newVol := round3(vol + float64(sign)*dv)
		if newCnt == 0 || newVol < 0 {
			newVol = 0
		}
		if _, err := tx.Exec(ctx, `UPDATE boards SET count = $1, volume_m3 = $2 WHERE id = $3`, newCnt, newVol, itemID); err != nil {
			return nil, err
		}
		adj.DeltaVolume = float64(sign) * dv
		adj.ItemLabel = fmt.Sprintf("%s · доска %d×%d · %s", species, h, w, lenLabel(l))
		if grade != "" {
			adj.ItemLabel += " · сорт " + grade
		}

	case "log":
		var l, d, arrival, rc int
		var species string
		var rv float64
		err := tx.QueryRow(ctx, `SELECT length_mm, diameter_mm, species, arrival_id, remaining_count, remaining_volume_m3 FROM log_items WHERE id = $1 FOR UPDATE`, itemID).
			Scan(&l, &d, &species, &arrival, &rc, &rv)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrLogNotFound
			}
			return nil, err
		}
		if count > rc {
			return nil, ErrInsufficientStock
		}
		dv := domain.ItemVolume(rc, rv, count)
		newVol := round3(rv - dv)
		if rc-count == 0 || newVol < 0 {
			newVol = 0
		}
		if _, err := tx.Exec(ctx, `UPDATE log_items SET remaining_count = $1, remaining_volume_m3 = $2 WHERE id = $3`, rc-count, newVol, itemID); err != nil {
			return nil, err
		}
		adj.DeltaVolume = -dv
		adj.ItemLabel = fmt.Sprintf("%s · бревно Ø%d · %s · партия №%d", species, d, lenLabel(l), arrival)

	default:
		return nil, errors.New("неизвестный вид позиции")
	}

	err = tx.QueryRow(ctx, `INSERT INTO stock_adjustments (kind, item_id, item_label, delta_count, delta_volume, reason, note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
		adj.Kind, adj.ItemID, adj.ItemLabel, adj.DeltaCount, adj.DeltaVolume, adj.Reason, adj.Note, userID).Scan(&adj.ID, &adj.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &adj, nil
}

func (r *Repository) ListStockAdjustments(ctx context.Context, limit, offset int) ([]domain.StockAdjustment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.kind, a.item_id, a.item_label, a.delta_count, a.delta_volume, a.reason, a.note,
		       COALESCE(a.created_by, 0), COALESCE(NULLIF(u.display_name, ''), u.username, ''), a.created_at
		FROM stock_adjustments a LEFT JOIN users u ON u.id = a.created_by
		ORDER BY a.id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.StockAdjustment, 0)
	for rows.Next() {
		var a domain.StockAdjustment
		if err := rows.Scan(&a.ID, &a.Kind, &a.ItemID, &a.ItemLabel, &a.DeltaCount, &a.DeltaVolume, &a.Reason, &a.Note, &a.CreatedBy, &a.UserName, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
