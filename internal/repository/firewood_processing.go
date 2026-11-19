package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
	"telegram-mini-app/internal/domain"
)

// Serialize consumption of the shared raw stock, including concurrent requests.
func (r *Repository) CreateFirewoodProcessing(ctx context.Context, orgID int64, p *domain.FirewoodProcessing, userID int64) error {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockFirewood(ctx, tx, orgID); err != nil {
		return err
	}
	stock, err := readFirewoodStock(ctx, orgID, tx)
	if err != nil {
		return err
	}
	if err := ensureFirewoodSettings(ctx, tx, orgID); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT coefficient FROM firewood_settings WHERE organization_id = $1 AND kind=$2 FOR SHARE`, orgID, p.Kind).Scan(&p.Coefficient); err != nil {
		return err
	}
	if p.VolumeM3*p.Coefficient > stock.RawM3+1e-9 {
		return ErrInsufficientStock
	}
	err = tx.QueryRow(ctx, `INSERT INTO firewood_processing(organization_id, kind,volume_m3,coefficient,raw_m3,performer,date,comment,created_by) VALUES($1, $2,$3,$4,$3::numeric*$4::numeric,$5,$6::date,$7,$8) RETURNING id,raw_m3`, orgID, p.Kind, p.VolumeM3, p.Coefficient, p.Performer, p.Date, p.Comment, userID).Scan(&p.ID, &p.RawM3)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) FirewoodProcessing(ctx context.Context, orgID int64, limit, offset int) ([]domain.FirewoodProcessing, error) {
	rows, err := r.db.Query(ctx, `SELECT id,kind,volume_m3,coefficient,raw_m3,performer,to_char(date,'YYYY-MM-DD'),comment FROM firewood_processing WHERE organization_id = $1 ORDER BY date DESC,id DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
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
func (r *Repository) DeleteFirewoodProcessing(ctx context.Context, orgID int64, id int64) error {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockFirewood(ctx, tx, orgID); err != nil {
		return err
	}
	var kind string
	var volume float64
	if err = tx.QueryRow(ctx, `SELECT kind,volume_m3 FROM firewood_processing WHERE organization_id = $1 AND id=$2`, orgID, id).Scan(&kind, &volume); err != nil {
		return err
	}
	if err = checkFirewoodOrderStock(ctx, tx, orgID, []domain.FirewoodOrderItem{{Kind: kind, VolumeM3: volume}}); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM firewood_processing WHERE organization_id = $1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}
func (r *Repository) FirewoodSettings(ctx context.Context, orgID int64) (map[string]float64, error) {
	rows, err := r.db.Query(ctx, `SELECT kind,coefficient FROM firewood_settings WHERE organization_id = $1 ORDER BY kind`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{"split": defaultSplitCoefficient, "round": defaultRoundCoefficient}
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

const (
	defaultSplitCoefficient = 0.55
	defaultRoundCoefficient = 0.60
)

func ensureFirewoodSettings(ctx context.Context, tx pgx.Tx, orgID int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO firewood_settings (organization_id, kind, coefficient)
		VALUES ($1, 'split', $2), ($1, 'round', $3) ON CONFLICT (organization_id, kind) DO NOTHING`,
		orgID, defaultSplitCoefficient, defaultRoundCoefficient)
	return err
}

func (r *Repository) UpdateFirewoodSettings(ctx context.Context, orgID int64, split, round float64) error {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockFirewood(ctx, tx, orgID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO firewood_settings (organization_id, kind, coefficient)
		VALUES ($1, 'split', $2), ($1, 'round', $3)
		ON CONFLICT (organization_id, kind) DO UPDATE SET coefficient = EXCLUDED.coefficient`, orgID, split, round)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
