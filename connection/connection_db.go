package connection

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		return nil, errors.New("DATABASE_URL не задан в переменных окружения")
	}

	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, errors.New("DATABASE_URL имеет неверный формат")
	}

	// Ограничения пула и защита от «зависших» запросов
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000" // 30 секунд на любой запрос

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
