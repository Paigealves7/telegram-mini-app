package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin_InvalidRequestBody(t *testing.T) {
	// Добавили второй аргумент - моковый секрет "test_secret"
	handler := NewAuthHandler(nil, "test_secret")

	// Отправляем некорректный JSON
	invalidJSON := []byte(`{"username": "admin", "password":}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(invalidJSON))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался статус %d, получен %d", http.StatusBadRequest, rec.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	// Проверяем, что middleware блокирует запросы без заголовка Authorization
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// AuthMiddleware теперь функция-фабрика:
	// сначала передаем моковый секрет, затем оборачиваем хендлер
	protectedHandler := AuthMiddleware("test_secret")(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/contractors", nil)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался статус %d для незащищенного запроса, получен %d", http.StatusUnauthorized, rec.Code)
	}
}
