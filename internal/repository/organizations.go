package repository

import (
	"context"
	"errors"
	"time"

	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
)

var (
	ErrOrganizationNotFound           = errors.New("организация не найдена или отключена")
	ErrOrganizationMembershipNotFound = errors.New("пользователь не состоит в активной организации")
	ErrInviteNotFound                 = errors.New("приглашение не найдено")
	ErrInviteUnavailable              = errors.New("приглашение отозвано, просрочено или исчерпано")
)

const organizationColumns = "id, name, company_info, is_demo, is_active, last_order_number, created_at"
const inviteColumns = "id, organization_id, role, max_uses, used_count, expires_at, revoked_at, created_by, created_at"

func organizationError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrganizationNotFound
	}
	return err
}

func lockOrgShare(ctx context.Context, tx pgx.Tx, orgID int64) error {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id = $1 AND is_active = TRUE FOR SHARE`, orgID).Scan(&id)
	return organizationError(err)
}

func (r *Repository) beginOrgWrite(ctx context.Context, orgID int64) (pgx.Tx, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := lockOrgShare(ctx, tx, orgID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func scanOrganization(row pgx.Row) (*domain.Organization, error) {
	var org domain.Organization
	err := row.Scan(&org.ID, &org.Name, &org.CompanyInfo, &org.IsDemo, &org.IsActive, &org.LastOrderNumber, &org.CreatedAt)
	if err != nil {
		return nil, organizationError(err)
	}
	return &org, nil
}

func (r *Repository) lockOrgExclusive(ctx context.Context, tx pgx.Tx, orgID int64) (*domain.Organization, error) {
	return scanOrganization(tx.QueryRow(ctx, `SELECT `+organizationColumns+` FROM organizations WHERE id = $1 FOR UPDATE`, orgID))
}

func (r *Repository) CreateOrganization(ctx context.Context, name, companyInfo string, isDemo bool) (*domain.Organization, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	org, err := scanOrganization(tx.QueryRow(ctx, `INSERT INTO organizations (name, company_info, is_demo)
		VALUES ($1, $2, $3) RETURNING `+organizationColumns, name, companyInfo, isDemo))
	if err != nil {
		return nil, err
	}
	if err := ensureFirewoodSettings(ctx, tx, org.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return org, nil
}

func (r *Repository) UpdateOrganization(ctx context.Context, id int64, name, companyInfo string, isDemo, isActive bool) (*domain.Organization, error) {
	return scanOrganization(r.db.QueryRow(ctx, `UPDATE organizations SET name = $2, company_info = $3, is_demo = $4, is_active = $5
		WHERE id = $1 RETURNING `+organizationColumns, id, name, companyInfo, isDemo, isActive))
}

func (r *Repository) GetOrganizationByID(ctx context.Context, id int64) (*domain.Organization, error) {
	return scanOrganization(r.db.QueryRow(ctx, `SELECT `+organizationColumns+` FROM organizations WHERE id = $1`, id))
}

func (r *Repository) ListOrganizations(ctx context.Context) ([]domain.Organization, error) {
	rows, err := r.db.Query(ctx, `SELECT `+organizationColumns+` FROM organizations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[domain.Organization])
}

func (r *Repository) GetPlatformOverview(ctx context.Context) (*domain.PlatformOverview, error) {
	overview := &domain.PlatformOverview{Organizations: []domain.OrganizationStats{}}
	started := time.Now()
	if err := r.db.Ping(ctx); err != nil {
		return nil, err
	}
	overview.DBLatencyMs = float64(time.Since(started)) / float64(time.Millisecond)

	if err := r.db.QueryRow(ctx, `SELECT pg_database_size(current_database()), (SELECT COUNT(*) FROM users)`).
		Scan(&overview.DBSizeBytes, &overview.TotalUsers); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `WITH members AS (
		SELECT organization_id, COUNT(*) AS count FROM organization_members GROUP BY organization_id
	), order_stats AS (
		SELECT organization_id, COUNT(*) AS count, MAX(created_at) AS last_activity
		FROM orders GROUP BY organization_id
	), board_stock AS (
		SELECT organization_id, SUM(volume_m3) AS volume
		FROM boards WHERE count > 0 GROUP BY organization_id
	), log_stock AS (
		SELECT organization_id, SUM(remaining_volume_m3) AS volume
		FROM log_items WHERE remaining_volume_m3 > 0 GROUP BY organization_id
	), log_activity AS (
		SELECT organization_id, MAX(created_at) AS last_activity FROM log_arrivals GROUP BY organization_id
	), sawing_activity AS (
		SELECT organization_id, MAX(created_at) AS last_activity FROM sawing_operations GROUP BY organization_id
	), sale_activity AS (
		SELECT organization_id, MAX(created_at) AS last_activity FROM sales GROUP BY organization_id
	)
	SELECT o.id, o.name, o.company_info, o.is_demo, o.is_active, o.last_order_number, o.created_at,
		COALESCE(members.count, 0), COALESCE(order_stats.count, 0),
		COALESCE(board_stock.volume, 0), COALESCE(log_stock.volume, 0),
		firewood.raw, firewood.split + firewood.round,
		GREATEST(order_stats.last_activity, log_activity.last_activity, sawing_activity.last_activity,
			sale_activity.last_activity, firewood.last_arrival_at)
	FROM organizations o
	LEFT JOIN members ON members.organization_id = o.id
	LEFT JOIN order_stats ON order_stats.organization_id = o.id
	LEFT JOIN board_stock ON board_stock.organization_id = o.id
	LEFT JOIN log_stock ON log_stock.organization_id = o.id
	LEFT JOIN log_activity ON log_activity.organization_id = o.id
	LEFT JOIN sawing_activity ON sawing_activity.organization_id = o.id
	LEFT JOIN sale_activity ON sale_activity.organization_id = o.id
	CROSS JOIN LATERAL (`+firewoodStockQuery+`) firewood
	ORDER BY o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var stats domain.OrganizationStats
		err := rows.Scan(&stats.ID, &stats.Name, &stats.CompanyInfo, &stats.IsDemo, &stats.IsActive,
			&stats.LastOrderNumber, &stats.CreatedAt, &stats.MembersCount, &stats.OrdersCount,
			&stats.BoardsVolumeM3, &stats.LogsVolumeM3, &stats.FirewoodRawM3, &stats.FirewoodReadyM3, &stats.LastActivityAt)
		if err != nil {
			return nil, err
		}
		overview.Organizations = append(overview.Organizations, stats)
		overview.TotalOrders += stats.OrdersCount
		if stats.IsActive {
			overview.ActiveOrganizations++
		}
		if stats.IsDemo {
			overview.DemoOrganizations++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	overview.TotalOrganizations = len(overview.Organizations)
	return overview, nil
}

func (r *Repository) ListUserOrganizations(ctx context.Context, userID int64) ([]domain.UserOrganization, error) {
	rows, err := r.db.Query(ctx, `SELECT o.id AS organization_id, o.name, o.company_info, o.is_demo, o.is_active, m.role
		FROM organization_members m JOIN organizations o ON o.id = m.organization_id
		WHERE m.user_id = $1 AND o.is_active = TRUE ORDER BY o.id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[domain.UserOrganization])
}

func (r *Repository) GetUserOrganizationRole(ctx context.Context, orgID, userID int64) (domain.Role, error) {
	var role domain.Role
	err := r.db.QueryRow(ctx, `SELECT m.role FROM organization_members m JOIN organizations o ON o.id = m.organization_id
		WHERE m.organization_id = $1 AND m.user_id = $2 AND o.is_active = TRUE`, orgID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOrganizationMembershipNotFound
	}
	return role, err
}

func (r *Repository) AddOrganizationMember(ctx context.Context, orgID, userID int64, role domain.Role) error {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, $3)`,
		orgID, userID, role)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) CreateInvite(ctx context.Context, orgID int64, codeHash string, role domain.Role, maxUses int, expiresAt time.Time, createdBy int64) (*domain.OrganizationInvite, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var invite domain.OrganizationInvite
	err = tx.QueryRow(ctx, `INSERT INTO organization_invites (organization_id, code_hash, role, max_uses, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6::bigint, 0)) RETURNING `+inviteColumns,
		orgID, codeHash, role, maxUses, expiresAt, createdBy).
		Scan(&invite.ID, &invite.OrganizationID, &invite.Role, &invite.MaxUses, &invite.UsedCount,
			&invite.ExpiresAt, &invite.RevokedAt, &invite.CreatedBy, &invite.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &invite, nil
}

func (r *Repository) ListOrganizationInvites(ctx context.Context, orgID int64) ([]domain.OrganizationInvite, error) {
	rows, err := r.db.Query(ctx, `SELECT `+inviteColumns+` FROM organization_invites
		WHERE organization_id = $1 AND COALESCE(revoked_at, expires_at) >= NOW() - INTERVAL '30 days'
		ORDER BY id DESC`, orgID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.OrganizationInvite])
}

const revokeInviteSQL = `UPDATE organization_invites SET revoked_at = NOW()
	WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL`

func (r *Repository) RevokeInvite(ctx context.Context, orgID, inviteID int64) (bool, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, revokeInviteSQL, inviteID, orgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

func (r *Repository) RevokeAdminInvite(ctx context.Context, orgID, inviteID int64) (bool, error) {
	// Одно обновление по паре (лесопилка, приглашение) работает и для замороженной лесопилки.
	tag, err := r.db.Exec(ctx, revokeInviteSQL, inviteID, orgID)
	return tag.RowsAffected() > 0, err
}

func (r *Repository) AcceptInvite(ctx context.Context, codeHash string, userID int64) (*domain.UserOrganization, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Блокировки берём в том же порядке, что при отзыве приглашения: организация, затем приглашение.
	var membership domain.UserOrganization
	err = tx.QueryRow(ctx, `SELECT organization_id FROM organization_invites WHERE code_hash = $1`, codeHash).
		Scan(&membership.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	orgID := membership.OrganizationID
	if err := lockOrgShare(ctx, tx, orgID); err != nil {
		return nil, err
	}

	var inviteID int64
	var maxUses, usedCount int
	var expiresAt time.Time
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id, role, max_uses, used_count, expires_at, revoked_at
		FROM organization_invites WHERE code_hash = $1 AND organization_id = $2 FOR UPDATE`, codeHash, orgID).
		Scan(&inviteID, &membership.Role, &maxUses, &usedCount, &expiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	if revokedAt != nil || usedCount >= maxUses {
		return nil, ErrInviteUnavailable
	}

	// Удаление сотрудника использует ту же блокировку аккаунта перед записью в список исключений.
	var username string
	if err := tx.QueryRow(ctx, `SELECT username FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&username); err != nil {
		return nil, err
	}
	var deleted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM deleted_telegram_users WHERE organization_id = $1 AND username = $2
	)`, orgID, username).Scan(&deleted); err != nil {
		return nil, err
	}
	if deleted {
		return nil, ErrDeletedUser
	}

	tag, err := tx.Exec(ctx, `UPDATE organization_invites SET used_count = used_count + 1
		WHERE id = $1 AND organization_id = $2 AND expires_at > clock_timestamp()`, inviteID, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrInviteUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role`, orgID, userID, membership.Role)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1 AND role = 'pending'`, userID, membership.Role); err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `SELECT name, company_info, is_demo, is_active FROM organizations WHERE id = $1`, orgID).
		Scan(&membership.Name, &membership.CompanyInfo, &membership.IsDemo, &membership.IsActive)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &membership, nil
}
