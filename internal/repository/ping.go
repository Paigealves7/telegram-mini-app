package repository

import "context"

// Ping проверяет, что база отвечает (для /readyz и мониторинга).
func (r *Repository) Ping(ctx context.Context) error {
	return r.db.Ping(ctx)
}
