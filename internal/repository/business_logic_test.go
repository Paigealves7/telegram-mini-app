package repository

import (
	"errors"
	"testing"

	"telegram-mini-app/internal/domain"
)

func TestValidateSawingYield(t *testing.T) {
	tests := []struct {
		name    string
		input   float64
		output  float64
		wantErr bool
	}{
		{name: "boards within input volume", input: 2.5, output: 2.4},
		{name: "rounding tolerance accepted", input: 2.5, output: 2.5004},
		{name: "boards exceed input volume", input: 2.5, output: 2.501, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSawingYield(tt.input, tt.output)
			if tt.wantErr && !errors.Is(err, ErrSawingYieldExceedsInput) {
				t.Fatalf("ожидали ErrSawingYieldExceedsInput, получили %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("не ожидали ошибку, получили %v", err)
			}
		})
	}
}

func TestValidateOrderStock(t *testing.T) {
	stocks := map[int64]boardStock{
		7: {count: 10, volume: 1.5},
		8: {count: 2, volume: 0.3},
	}
	tests := []struct {
		name    string
		items   []domain.OrderItem
		wantErr error
	}{
		{
			name:  "requested count within stock",
			items: []domain.OrderItem{{BoardID: 7, Count: 6}, {BoardID: 8, Count: 2}},
		},
		{
			name:    "one order line exceeds stock",
			items:   []domain.OrderItem{{BoardID: 7, Count: 11}},
			wantErr: ErrInsufficientStock,
		},
		{
			name:    "duplicate board lines summed against stock",
			items:   []domain.OrderItem{{BoardID: 7, Count: 6}, {BoardID: 7, Count: 5}},
			wantErr: ErrInsufficientStock,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requested := requestedOrderCounts(tt.items)
			gotErr := validateOrderStock(requested, stocks)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Fatalf("ошибка = %v, ожидали %v", gotErr, tt.wantErr)
			}
		})
	}
}
