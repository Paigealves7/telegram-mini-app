package repository

import (
	"context"

	"telegram-mini-app/internal/domain"
)

// GetOrderLines возвращает позиции заказа с названием и размерами досок.
// LEFT JOIN + COALESCE: накладная строится, даже если доска когда-то пропала со склада.
func (r *Repository) GetOrderLines(ctx context.Context, orderID int64) ([]domain.OrderLine, error) {
	const q = `
		SELECT ob.board_id,
		       COALESCE(b.species, ''), COALESCE(b.grade, ''),
		       COALESCE(b.height_mm, 0), COALESCE(b.width_mm, 0), COALESCE(b.length_mm, 0),
		       ob.count, COALESCE(ob.volume_m3, 0), COALESCE(ob.price, 0)
		FROM order_boards ob
		LEFT JOIN boards b ON b.id = ob.board_id
		WHERE ob.order_id = $1
		ORDER BY ob.id`

	rows, err := r.db.Query(ctx, q, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []domain.OrderLine
	for rows.Next() {
		var l domain.OrderLine
		if err := rows.Scan(&l.BoardID, &l.Species, &l.Grade, &l.HeightMM, &l.WidthMM, &l.LengthMM, &l.Count, &l.VolumeM3, &l.Price); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

// GetContractorName возвращает название клиента.
func (r *Repository) GetContractorName(ctx context.Context, id int64) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `SELECT name FROM contractors WHERE id = $1`, id).Scan(&name)
	return name, err
}
