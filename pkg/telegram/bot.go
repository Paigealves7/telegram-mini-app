package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// BotUsername спрашивает у Telegram (getMe), какому боту принадлежит токен.
// Ошибки намеренно без URL: в нём лежит токен.
func BotUsername(ctx context.Context, botToken string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/bot"+botToken+"/getMe", nil)
	if err != nil {
		return "", errors.New("telegram: не удалось собрать запрос getMe")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.New("telegram: запрос getMe не прошёл (сеть или DNS)")
	}
	defer resp.Body.Close()

	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("telegram: непонятный ответ getMe (HTTP %d)", resp.StatusCode)
	}
	if !out.OK {
		return "", fmt.Errorf("telegram: getMe отклонил токен (HTTP %d)", resp.StatusCode)
	}
	return out.Result.Username, nil
}
