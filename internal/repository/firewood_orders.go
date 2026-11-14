package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
	"telegram-mini-app/internal/domain"
)

// Called under the shared firewood lock. Pending orders do not reserve stock.
func checkFirewoodOrderStock(ctx context.Context, tx pgx.Tx, items []domain.FirewoodOrderItem) error {
	requested := map[string]float64{}
	for _, p := range items {
		requested[p.Kind] += p.VolumeM3
	}
	for kind, volume := range requested {
		var stock float64
		err := tx.QueryRow(ctx, `SELECT (SELECT COALESCE(SUM(volume_m3),0) FROM firewood_processing WHERE kind=$1)-(SELECT COALESCE(SUM(f.volume_m3),0) FROM order_firewood f JOIN orders o ON o.id=f.order_id WHERE f.kind=$1 AND o.status='completed')`, kind).Scan(&stock)
		if err != nil {
			return err
		}
		if volume > stock+1e-9 {
			return ErrInsufficientStock
		}
	}
	return nil
}
func firewoodOrderItems(ctx context.Context, tx pgx.Tx, id int64) ([]domain.FirewoodOrderItem, error) {
	rows, err := tx.Query(ctx, `SELECT kind,volume_m3,price FROM order_firewood WHERE order_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FirewoodOrderItem, 0)
	for rows.Next() {
		var p domain.FirewoodOrderItem
		if err = rows.Scan(&p.Kind, &p.VolumeM3, &p.Price); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
