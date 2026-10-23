package service

import "testing"

func TestChatUnavailable(t *testing.T) {
	tests := []struct {
		status int
		desc   string
		want   bool
	}{
		{403, "Forbidden: bot was blocked by the user", true},
		{400, "Bad Request: chat not found", true},
		{400, "Bad Request: can't initiate conversation with a user", true},
		{403, "", true},
		{400, "Bad Request: message text is empty", false},
		{500, "Internal Server Error", false},
	}
	for _, tt := range tests {
		if got := chatUnavailable(tt.status, tt.desc); got != tt.want {
			t.Errorf("chatUnavailable(%d, %q) = %v, ожидали %v", tt.status, tt.desc, got, tt.want)
		}
	}
}
