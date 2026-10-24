package http

import (
	"context"
	"log"
	"net/http"
	"time"
)

type pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db pinger
}

func NewHealthHandler(db pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Live: процесс жив и отвечает. База не проверяется.
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok"))
}

// Ready: приложение способно работать, то есть база отвечает за 2 секунды.
// Подробности ошибки в ответ не попадают, только в лог.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		log.Printf("READY: база недоступна: %v", err)
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}
