package http

import (
	"net/http"
	"time"

	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

type poolStats struct {
	Acquired int32 `json:"acquired_connections"`
	Idle     int32 `json:"idle_connections"`
	Total    int32 `json:"total_connections"`
	Max      int32 `json:"max_connections"`
}

func platformOverview(repo *repository.Repository, db *pgxpool.Pool, startedAt time.Time, botUsername string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		overview, err := repo.GetPlatformOverview(r.Context())
		if err != nil {
			serverError(w, r, "не удалось получить сводку платформы", err)
			return
		}
		overview.UptimeSeconds = int64(time.Since(startedAt) / time.Second)
		out := struct {
			*domain.PlatformOverview
			DBPool      *poolStats `json:"db_pool,omitempty"`
			BotUsername string     `json:"bot_username,omitempty"`
		}{PlatformOverview: overview, BotUsername: botUsername}
		if db != nil {
			stat := db.Stat()
			out.DBPool = &poolStats{Acquired: stat.AcquiredConns(), Idle: stat.IdleConns(),
				Total: stat.TotalConns(), Max: stat.MaxConns()}
		}
		pdfWriteJSON(w, http.StatusOK, out)
	}
}
