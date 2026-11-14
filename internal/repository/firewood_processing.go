package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
	"telegram-mini-app/internal/domain"
)

// Serialize consumption of the shared raw stock, including concurrent requests.
func (r *Repository) CreateFirewoodProcessing(ctx context.Context, p *domain.FirewoodProcessing, userID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(900009)`); err != nil {
		return err
	}
	var available float64
	if err = tx.QueryRow(ctx, `SELECT (SELECT COALESCE(SUM(volume_m3),0) FROM firewood_arrivals) - (SELECT COALESCE(SUM(raw_m3),0) FROM firewood_processing)`).Scan(&available); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT coefficient FROM firewood_settings WHERE kind=$1 FOR SHARE`, p.Kind).Scan(&p.Coefficient); err != nil {
		return err
	}
	if p.VolumeM3*p.Coefficient > available+1e-9 {
		return ErrInsufficientStock
	}
	err = tx.QueryRow(ctx, `INSERT INTO firewood_processing(kind,volume_m3,coefficient,raw_m3,performer,date,comment,created_by) VALUES($1,$2,$3,$2::numeric*$3::numeric,$4,$5::date,$6,$7) RETURNING id,raw_m3`, p.Kind, p.VolumeM3, p.Coefficient, p.Performer, p.Date, p.Comment, userID).Scan(&p.ID, &p.RawM3)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) FirewoodProcessing(ctx context.Context, limit, offset int) ([]domain.FirewoodProcessing, error) {
	rows, err := r.db.Query(ctx, `SELECT id,kind,volume_m3,coefficient,raw_m3,performer,to_char(date,'YYYY-MM-DD'),comment FROM firewood_processing ORDER BY date DESC,id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.FirewoodProcessing, 0)
	for rows.Next() {
		var p domain.FirewoodProcessing
		if err = rows.Scan(&p.ID, &p.Kind, &p.VolumeM3, &p.Coefficient, &p.RawM3, &p.Performer, &p.Date, &p.Comment); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r *Repository) DeleteFirewoodProcessing(ctx context.Context, id int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(900009)`); err != nil {
		return err
	}
	var kind string
	var volume float64
	if err = tx.QueryRow(ctx, `SELECT kind,volume_m3 FROM firewood_processing WHERE id=$1`, id).Scan(&kind, &volume); err != nil {
		return err
	}
	if err = checkFirewoodOrderStock(ctx, tx, []domain.FirewoodOrderItem{{Kind: kind, VolumeM3: volume}}); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM firewood_processing WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}
func (r *Repository) FirewoodSettings(ctx context.Context) (map[string]float64, error) {
	rows, err := r.db.Query(ctx, `SELECT kind,coefficient FROM firewood_settings ORDER BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]float64)
	for rows.Next() {
		var k string
		var c float64
		if err = rows.Scan(&k, &c); err != nil {
			return nil, err
		}
		out[k] = c
	}
	return out, rows.Err()
}
func (r *Repository) UpdateFirewoodSettings(ctx context.Context, split, round float64) error {
	_, err := r.db.Exec(ctx, `UPDATE firewood_settings SET coefficient=CASE kind WHEN 'split' THEN $1::numeric ELSE $2::numeric END`, split, round)
	return err
}
