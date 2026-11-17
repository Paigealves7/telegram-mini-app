package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// BotHTTPClient настраивает общий доступ к Bot API для диагностики и отправки.
func BotHTTPClient(timeout time.Duration) (*http.Client, string, error) {
	base := strings.TrimSpace(os.Getenv("TELEGRAM_API_BASE"))
	if base == "" {
		base = "https://api.telegram.org"
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", errors.New("некорректный TELEGRAM_API_BASE")
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	if raw := strings.TrimSpace(os.Getenv("TELEGRAM_PROXY")); raw != "" {
		p, err := url.Parse(raw)
		if err != nil || p.Hostname() == "" || (p.Scheme != "http" && p.Scheme != "https" && p.Scheme != "socks5" && p.Scheme != "socks5h") {
			return nil, "", errors.New("некорректный TELEGRAM_PROXY")
		}
		transport := &http.Transport{}
		if defaults, ok := http.DefaultTransport.(*http.Transport); ok {
			transport = defaults.Clone()
		}
		transport.Proxy = http.ProxyURL(p)
		client.Transport = transport
	}
	return client, strings.TrimRight(base, "/"), nil
}

// BotUsername спрашивает у Telegram (getMe), какому боту принадлежит токен.
// Ошибки намеренно без URL: в нём лежит токен.
func BotUsername(ctx context.Context, botToken string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client, base, err := BotHTTPClient(5 * time.Second)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/bot"+botToken+"/getMe", nil)
	if err != nil {
		return "", errors.New("telegram: не удалось собрать запрос getMe")
	}

	resp, err := client.Do(req)
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
