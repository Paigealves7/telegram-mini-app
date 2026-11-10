package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(ctx context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	w := httptest.NewRecorder()
	NewHealthHandler(fakePinger{}).Ready(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Errorf("ready при живой базе: код %d, тело %q", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	NewHealthHandler(fakePinger{err: errors.New("connection refused")}).Ready(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("ready при упавшей базе должен дать 503, получили %d", w.Code)
	}
	if body := w.Body.String(); len(body) > 30 {
		t.Errorf("в ответ не должны попадать подробности ошибки: %q", body)
	}

	// liveness не зависит от базы
	w = httptest.NewRecorder()
	NewHealthHandler(fakePinger{err: errors.New("down")}).Live(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("live не должен зависеть от базы, код %d", w.Code)
	}
}
