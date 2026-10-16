package http

import (
    "net/http"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// RateLimiter — простой in-memory rate limiter по IP.
// Для продакшена с несколькими инстансами лучше использовать Redis.
type RateLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    r        rate.Limit
    b        int
}

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
    return &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        r:        rate.Limit(float64(requestsPerMinute) / 60.0), // в секунду
        b:        requestsPerMinute,
    }
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    limiter, exists := rl.limiters[ip]
    if !exists {
        limiter = rate.NewLimiter(rl.r, rl.b)
        rl.limiters[ip] = limiter
    }
    return limiter
}

// Middleware — оборачивает handler ограничением по IP.
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Получаем реальный IP
        ip := r.Header.Get("X-Real-IP")
        if ip == "" {
            ip = r.Header.Get("X-Forwarded-For")
        }
        if ip == "" {
            ip = r.RemoteAddr
        }

        limiter := rl.getLimiter(ip)
        if !limiter.Allow() {
            http.Error(w, "слишком много запросов, попробуйте позже", http.StatusTooManyRequests)
            return
        }

        next(w, r)
    }
}

// CleanupOldLimiters — очистка старых записей (запускать раз в час).
func (rl *RateLimiter) CleanupOldLimiters() {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    // Простая стратегия: очищаем всё раз в час.
    // Для точной работы — надо хранить lastSeen на каждый IP.
    rl.limiters = make(map[string]*rate.Limiter)
}

// StartCleanup — запускает фоновую очистку.
func (rl *RateLimiter) StartCleanup() {
    go func() {
        ticker := time.NewTicker(1 * time.Hour)
        defer ticker.Stop()
        for range ticker.C {
            rl.CleanupOldLimiters()
        }
    }()
}
