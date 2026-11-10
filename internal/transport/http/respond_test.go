package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"telegram-mini-app/internal/repository"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidationMessage(t *testing.T) {
	err := Validate.Struct(CreateOrderRequest{})
	msg := validationMessage(err)
	if !strings.Contains(msg, "клиент") || strings.Contains(msg, "Key:") {
		t.Errorf("сообщение должно быть понятным и без служебного текста: %q", msg)
	}
}

func TestParseDate(t *testing.T) {
	for _, ok := range []string{"2026-10-21", "2000-01-01"} {
		if _, err := parseDate(ok); err != nil {
			t.Errorf("%s должна приниматься: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "21.10.2026", "2026-13-01", "2026-02-30", "1999-12-31", "2101-01-01", "abc"} {
		if _, err := parseDate(bad); err == nil {
			t.Errorf("%q должна отклоняться", bad)
		}
	}
}

func TestPageParams(t *testing.T) {
	tests := []struct {
		q                     string
		wantLimit, wantOffset int
	}{
		{"", 100, 0},
		{"limit=20&offset=40", 20, 40},
		{"limit=999999", 500, 0},
		{"limit=-5&offset=-1", 100, 0},
		{"limit=abc", 100, 0},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/x?"+tt.q, nil)
		l, o := pageParams(r, 100, 500)
		if l != tt.wantLimit || o != tt.wantOffset {
			t.Errorf("%q: получили %d/%d, ожидали %d/%d", tt.q, l, o, tt.wantLimit, tt.wantOffset)
		}
	}
}

func TestRespondError(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	tests := []struct {
		name     string
		err      error
		wantCode int
		leak     string // этого текста в ответе быть не должно
	}{
		{"нет товара", repository.ErrInsufficientStock, http.StatusConflict, ""},
		{"нет доски", repository.ErrBoardNotFound, http.StatusBadRequest, ""},
		{"внешний ключ", &pgconn.PgError{Code: "23503", Message: "violates foreign key constraint orders_contractor_id_fkey"}, http.StatusBadRequest, "constraint"},
		{"дубль", &pgconn.PgError{Code: "23505", Message: "duplicate key value"}, http.StatusConflict, "duplicate"},
		{"неверная дата", &pgconn.PgError{Code: "22007", Message: "invalid input syntax for type date"}, http.StatusBadRequest, "syntax"},
		{"сбой сервера", errors.New("dial tcp 10.0.0.5:5432: connection refused"), http.StatusInternalServerError, "10.0.0.5"},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		respondError(w, r, "не удалось сохранить", tt.err)
		if w.Code != tt.wantCode {
			t.Errorf("%s: код %d, ожидали %d", tt.name, w.Code, tt.wantCode)
		}
		if tt.leak != "" && strings.Contains(w.Body.String(), tt.leak) {
			t.Errorf("%s: в ответ попали внутренние детали: %q", tt.name, w.Body.String())
		}
	}
}
