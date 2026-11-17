package repository

import (
	"context"

	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
)

func lockFirewood(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(900009)`)
	return err
}

// CreateFirewoodArrival записывает приход дров (общий объём в м³).
func (r *Repository) CreateFirewoodArrival(ctx context.Context, supplierID, carrierID int64, truck string, volume float64, date string, userID int64) (*domain.FirewoodArrival, error) {
	a := domain.FirewoodArrival{SupplierID: supplierID, CarrierID: carrierID, TruckNumber: truck, VolumeM3: volume}
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockFirewood(ctx, tx); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
		INSERT INTO firewood_arrivals (supplier_id, carrier_id, truck_number, volume_m3, arrival_date, created_by)
		VALUES ($1, $2, $3, $4, $5::date, $6)
		RETURNING id, to_char(arrival_date, 'YYYY-MM-DD'), created_at`,
			supplierID, carrierID, truck, volume, date, userID).Scan(&a.ID, &a.ArrivalDate, &a.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) GetFirewoodArrivals(ctx context.Context, limit, offset int) ([]domain.FirewoodArrival, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, supplier_id, carrier_id, truck_number, volume_m3, to_char(arrival_date, 'YYYY-MM-DD'), created_at
		FROM firewood_arrivals ORDER BY arrival_date DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.FirewoodArrival, 0)
	for rows.Next() {
		var a domain.FirewoodArrival
		if err := rows.Scan(&a.ID, &a.SupplierID, &a.CarrierID, &a.TruckNumber, &a.VolumeM3, &a.ArrivalDate, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetFirewoodStock считает остатки дров. Сырьё = приходы минус списания на переработку.
func (r *Repository) GetFirewoodStock(ctx context.Context) (*domain.FirewoodStock, error) {
	return readFirewoodStock(ctx, r.db)
}

func readFirewoodStock(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (*domain.FirewoodStock, error) {
	var s domain.FirewoodStock
	err := db.QueryRow(ctx, `WITH adjustments AS (
		SELECT COALESCE(SUM(raw_delta_m3),0) AS raw, COALESCE(SUM(split_delta_m3),0) AS split,
			COALESCE(SUM(round_delta_m3),0) AS round FROM firewood_stock_adjustments
	), production AS (
		SELECT COALESCE(SUM(raw_m3),0) AS raw, COALESCE(SUM(volume_m3) FILTER (WHERE kind='split'),0) AS split,
			COALESCE(SUM(volume_m3) FILTER (WHERE kind='round'),0) AS round FROM firewood_processing
	), shipped AS (
		SELECT COALESCE(SUM(f.volume_m3) FILTER (WHERE f.kind='split'),0) AS split,
			COALESCE(SUM(f.volume_m3) FILTER (WHERE f.kind='round'),0) AS round
		FROM order_firewood f JOIN orders o ON o.id=f.order_id WHERE o.status='completed'
	)
	SELECT (SELECT COALESCE(SUM(volume_m3),0) FROM firewood_arrivals)-p.raw+a.raw,
		p.split-s.split+a.split, p.round-s.round+a.round
	FROM production p CROSS JOIN adjustments a CROSS JOIN shipped s`).Scan(&s.RawM3, &s.SplitM3, &s.RoundM3)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) DeleteFirewoodArrival(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockFirewood(ctx, tx); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM firewood_arrivals WHERE id=$1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		stock, err := readFirewoodStock(ctx, tx)
		if err != nil {
			return err
		}
		if stock.RawM3 < 0 {
			return ErrInsufficientStock
		}
		return nil
	})
}

func (r *Repository) SetFirewoodStock(ctx context.Context, target domain.FirewoodStock, actorID int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockFirewood(ctx, tx); err != nil {
			return err
		}
		current, err := readFirewoodStock(ctx, tx)
		if err != nil {
			return err
		}
		if target == *current {
			return nil
		}
		_, err = tx.Exec(ctx, `INSERT INTO firewood_stock_adjustments
			(raw_delta_m3, split_delta_m3, round_delta_m3, created_by) VALUES ($1,$2,$3,$4)`,
			target.RawM3-current.RawM3, target.SplitM3-current.SplitM3, target.RoundM3-current.RoundM3, actorID)
		return err
	})
}
