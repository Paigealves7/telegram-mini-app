package repository

import (
	"errors"
	"strings"
	"testing"
)

func TestConsumeLog(t *testing.T) {
	tests := []struct {
		name         string
		remVol       float64
		remCount     int
		used         float64
		wantVol      float64
		wantCount    int
		wantInsuffic bool
	}{
		{"весь объём", 10, 20, 10, 0, 0, false},
		{"половина", 10, 20, 5, 5, 10, false},
		{"остаток в пределах допуска считается нулём", 10, 20, 9.9996, 0, 0, false},
		{"превышение в пределах допуска", 10, 20, 10.0004, 0, 0, false},
		{"в партии остаётся хотя бы одно бревно", 3, 3, 2.9, 0.1, 1, false},
		{"пропорциональный пересчёт штук", 4, 7, 1, 3, 5, false},
		{"ещё один пропорциональный пересчёт", 2, 10, 0.4, 1.6, 8, false},
		{"нельзя распилить больше остатка", 10, 20, 10.01, 0, 0, true},
		{"из пустой партии нельзя", 0, 0, 0.5, 0, 0, true},
	}
	for _, tt := range tests {
		vol, cnt, err := consumeLog(tt.remVol, tt.remCount, tt.used)
		if tt.wantInsuffic {
			if !errors.Is(err, ErrInsufficientLogs) {
				t.Errorf("%s: ожидали ErrInsufficientLogs, получили %v", tt.name, err)
			}
			continue
		}
		if err != nil || vol != tt.wantVol || cnt != tt.wantCount {
			t.Errorf("%s: получили (%v, %d, %v), ожидали (%v, %d)", tt.name, vol, cnt, err, tt.wantVol, tt.wantCount)
		}
	}

	_, _, err := consumeLog(10, 20, 11)
	if err == nil || !strings.Contains(err.Error(), "10,000") {
		t.Errorf("в сообщении должен быть остаток с запятой: %v", err)
	}
}
