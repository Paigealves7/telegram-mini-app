package repository

import (
	"context"

	"telegram-mini-app/internal/domain"
)

// CreateFirewoodArrival записывает приход дров (общий объём в м³).
func (r *Repository) CreateFirewoodArrival(ctx context.Context, supplierID, carrierID int64, truck string, volume float64, date string, userID int64) (*domain.FirewoodArrival, error) {
	a := domain.FirewoodArrival{SupplierID: supplierID, CarrierID: carrierID, TruckNumber: truck, VolumeM3: volume}
	err := r.db.QueryRow(ctx, `
		INSERT INTO firewood_arrivals (supplier_id, carrier_id, truck_number, volume_m3, arrival_date, created_by)
		VALUES ($1, $2, $3, $4, $5::date, $6)
		RETURNING id, to_char(arrival_date, 'YYYY-MM-DD'), created_at`,
		supplierID, carrierID, truck, volume, date, userID).Scan(&a.ID, &a.ArrivalDate, &a.CreatedAt)
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
	var s domain.FirewoodStock
	err := r.db.QueryRow(ctx, `SELECT (SELECT COALESCE(SUM(volume_m3),0) FROM firewood_arrivals)-(SELECT COALESCE(SUM(raw_m3),0) FROM firewood_processing), COALESCE(SUM(volume_m3) FILTER (WHERE kind='split'),0)-(SELECT COALESCE(SUM(f.volume_m3),0) FROM order_firewood f JOIN orders o ON o.id=f.order_id WHERE f.kind='split' AND o.status='completed'), COALESCE(SUM(volume_m3) FILTER (WHERE kind='round'),0)-(SELECT COALESCE(SUM(f.volume_m3),0) FROM order_firewood f JOIN orders o ON o.id=f.order_id WHERE f.kind='round' AND o.status='completed') FROM firewood_processing`).Scan(&s.RawM3, &s.SplitM3, &s.RoundM3)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
