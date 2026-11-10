package http

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type limiterEntry struct {
	lim    *rate.Limiter
	seen   time.Time
	warned time.Time
}

// RateLimiter — ограничитель запросов по IP в памяти (один инстанс приложения).
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	rate    rate.Limit
	burst   int
	maxKeys int
}

// NewRateLimiter: perMinute — устойчивая скорость, burst — сколько запросов подряд пропускается сразу.
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*limiterEntry),
		rate:    rate.Limit(float64(perMinute) / 60.0),
		burst:   burst,
		maxKeys: 50000,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		rl.cleanup(time.Now())
	}
}

// cleanup удаляет адреса, которых не видели 15 минут (раньше карта очищалась целиком раз в час).
func (rl *RateLimiter) cleanup(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, e := range rl.entries {
		if now.Sub(e.seen) > 15*time.Minute {
			delete(rl.entries, k)
		}
	}
}

// check решает, пропустить ли запрос. При отказе возвращает, через сколько можно повторить.
func (rl *RateLimiter) check(key string, now time.Time) (ok bool, retry time.Duration, warn bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, exists := rl.entries[key]
	if !exists {
		if len(rl.entries) >= rl.maxKeys {
			rl.entries = make(map[string]*limiterEntry) // защита памяти при атаке с огромным числом адресов
		}
		e = &limiterEntry{lim: rate.NewLimiter(rl.rate, rl.burst)}
		rl.entries[key] = e
	}
	e.seen = now

	res := e.lim.ReserveN(now, 1)
	if !res.OK() {
		return false, time.Minute, false
	}
	if d := res.DelayFrom(now); d > 0 {
		res.CancelAt(now) // отказ не должен «съедать» токен
		if now.Sub(e.warned) > 30*time.Second {
			e.warned = now
			warn = true
		}
		return false, d, warn
	}
	return true, 0, false
}

// Wrap подходит и для r.Use(...), и для обёртки отдельного маршрута.
func (rl *RateLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		ok, retry, warn := rl.check(ip, time.Now())
		if !ok {
			if warn {
				log.Printf("RATE LIMIT: превышен лимит (%s) от %s", r.Method, ip)
			}
			secs := int(math.Ceil(retry.Seconds()))
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			http.Error(w, fmt.Sprintf("слишком много запросов, повторите через %d сек.", secs), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return rl.Wrap(next).ServeHTTP
}
