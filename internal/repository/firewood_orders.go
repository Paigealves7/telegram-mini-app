package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
	"telegram-mini-app/internal/domain"
)

// Called under the shared firewood lock. Pending orders do not reserve stock.
func checkFirewoodOrderStock(ctx context.Context, tx pgx.Tx, orgID int64, items []domain.FirewoodOrderItem) error {
	requested := map[string]float64{}
	for _, p := range items {
		requested[p.Kind] += p.VolumeM3
	}
	if len(requested) == 0 {
		return nil
	}
	stock, err := readFirewoodStock(ctx, orgID, tx)
	if err != nil {
		return err
	}
	if requested["split"] > 0 && requested["split"] > stock.SplitM3+1e-9 ||
		requested["round"] > 0 && requested["round"] > stock.RoundM3+1e-9 {
		return ErrInsufficientStock
	}
	return nil
}
func firewoodOrderItems(ctx context.Context, tx pgx.Tx, orgID int64, id int64) ([]domain.FirewoodOrderItem, error) {
	rows, err := tx.Query(ctx, `SELECT kind,volume_m3,price FROM order_firewood WHERE organization_id = $1 AND order_id=$2 ORDER BY id`, orgID, id)
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
