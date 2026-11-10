package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func reqFrom(remote string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"прокси из частной сети, X-Real-IP", reqFrom("172.18.0.1:5555", map[string]string{"X-Real-IP": "203.0.113.7"}), "203.0.113.7"},
		{"прямой запрос из интернета: заголовок подделан", reqFrom("198.51.100.9:4444", map[string]string{"X-Real-IP": "1.2.3.4"}), "198.51.100.9"},
		{"X-Forwarded-For: берём последний", reqFrom("172.18.0.1:5555", map[string]string{"X-Forwarded-For": "9.9.9.9, 203.0.113.8"}), "203.0.113.8"},
		{"мусор в заголовке", reqFrom("172.18.0.1:5555", map[string]string{"X-Real-IP": "не-ip"}), "172.18.0.1"},
		{"localhost без заголовков", reqFrom("[::1]:1234", nil), "::1"},
	}
	for _, tt := range tests {
		if got := clientIP(tt.r); got != tt.want {
			t.Errorf("%s: получили %q, ожидали %q", tt.name, got, tt.want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(6, 3) // 3 подряд, дальше 1 в 10 секунд
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	call := func(remote string, headers map[string]string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h(w, reqFrom(remote, headers))
		return w
	}

	for i := 0; i < 3; i++ {
		if w := call("198.51.100.9:1", nil); w.Code != http.StatusOK {
			t.Fatalf("запрос %d должен пройти, код %d", i+1, w.Code)
		}
	}
	w := call("198.51.100.9:2", nil) // другой порт: тот же адрес, тот же лимит
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("4-й запрос должен получить 429 с Retry-After, код %d", w.Code)
	}
	// подмена заголовка с публичного адреса лимит не обходит
	if w := call("198.51.100.9:3", map[string]string{"X-Real-IP": "7.7.7.7"}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("подмена X-Real-IP не должна обходить лимит, код %d", w.Code)
	}
	// другой клиент не пострадал
	if w := call("198.51.100.77:1", nil); w.Code != http.StatusOK {
		t.Fatalf("другой адрес не должен блокироваться, код %d", w.Code)
	}
}
