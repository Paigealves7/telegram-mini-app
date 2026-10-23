package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"telegram-mini-app/internal/repository"
	"time"
)

var (
	ErrBotCannotWrite    = errors.New("бот не может написать вам: откройте чат с ботом и нажмите «Запустить» (Start), затем повторите")
	ErrNoTelegramAccount = errors.New("отправка в Telegram доступна только при входе через Telegram")
)

type BotService struct {
	botToken string
	repo     *repository.Repository
	client   *http.Client
}

func NewBotService(token string, repo *repository.Repository) *BotService {
	// Таймаут обязателен: без него зависший Telegram держит горутину и соединение бесконечно.
	return &BotService{botToken: token, repo: repo, client: &http.Client{Timeout: 20 * time.Second}}
}

type tgResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// chatUnavailable: пользователь не запускал бота, заблокировал его или удалил аккаунт.
func chatUnavailable(status int, description string) bool {
	if status == http.StatusForbidden {
		return true
	}
	d := strings.ToLower(description)
	for _, s := range []string{"chat not found", "can't initiate", "bot was blocked", "user is deactivated"} {
		if strings.Contains(d, s) {
			return true
		}
	}
	return false
}

// call — один запрос к Bot API. Тексты ошибок без URL: в нём токен бота.
func (s *BotService) call(ctx context.Context, method, contentType string, body []byte) (*tgResponse, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+s.botToken+"/"+method, bytes.NewReader(body))
	if err != nil {
		return nil, 0, errors.New("не удалось подготовить запрос к Telegram")
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, errors.New("Telegram недоступен с сервера (сеть, DNS или таймаут)")
	}
	defer resp.Body.Close()

	var out tgResponse
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out)
	return &out, resp.StatusCode, nil
}

// send повторяет запрос при сбоях сети, ошибках 5xx и лимите 429 (до 3 попыток).
// Постоянные ошибки (400 и т.п.) не повторяются.
func (s *BotService) send(ctx context.Context, method, contentType string, body []byte) error {
	const attempts = 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		out, status, err := s.call(ctx, method, contentType, body)
		wait := time.Duration(attempt) * time.Second
		switch {
		case err != nil:
			lastErr = err
		case out.OK:
			return nil
		case chatUnavailable(status, out.Description):
			return ErrBotCannotWrite
		case status == http.StatusTooManyRequests:
			lastErr = errors.New("Telegram просит снизить частоту запросов")
			if ra := out.Parameters.RetryAfter; ra > 0 {
				wait = time.Duration(min(ra, 10)) * time.Second
			}
		case status >= 500:
			lastErr = fmt.Errorf("Telegram вернул ошибку %d", status)
		default:
			return fmt.Errorf("Telegram отклонил запрос (HTTP %d): %s", status, out.Description)
		}
		if attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// chatFor возвращает chat_id пользователя, вошедшего через Telegram (логин tg_<id>).
func (s *BotService) chatFor(ctx context.Context, userID int64) (string, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(user.Username, "tg_") {
		return "", ErrNoTelegramAccount
	}
	chatID := strings.TrimPrefix(user.Username, "tg_")
	if _, err := strconv.ParseInt(chatID, 10, 64); err != nil {
		return "", ErrNoTelegramAccount
	}
	return chatID, nil
}

// NotifyUser отправляет сообщение. Для веб-пользователей (вход по паролю) тихо ничего не делает.
func (s *BotService) NotifyUser(ctx context.Context, userID int64, message string) error {
	chatID, err := s.chatFor(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNoTelegramAccount) {
			return nil
		}
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     message,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	return s.send(ctx, "sendMessage", "application/json", body)
}

// NotifyAsync отправляет уведомление в фоне, не задерживая ответ пользователю.
// Таймаут и recover обязательны: паника в горутине роняет весь процесс.
func (s *BotService) NotifyAsync(userID int64, message string) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("BOT: паника при отправке уведомления: %v", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		switch err := s.NotifyUser(ctx, userID, message); {
		case err == nil:
		case errors.Is(err, ErrBotCannotWrite):
			log.Printf("BOT: пользователь %d не запускал бота или заблокировал его, уведомление пропущено", userID)
		default:
			log.Printf("BOT: уведомление пользователю %d не доставлено: %v", userID, err)
		}
	}()
}

// SendDocument отправляет файл пользователю в личный чат с ботом.
func (s *BotService) SendDocument(ctx context.Context, userID int64, filename string, data []byte, caption string) error {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	chatID, err := s.chatFor(ctx, userID)
	if err != nil {
		return err
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("chat_id", chatID)
	_ = mw.WriteField("caption", caption)
	part, err := mw.CreateFormFile("document", filename)
	if err != nil {
		return errors.New("не удалось подготовить файл для Telegram")
	}
	if _, err := part.Write(data); err != nil {
		return errors.New("не удалось подготовить файл для Telegram")
	}
	if err := mw.Close(); err != nil {
		return errors.New("не удалось подготовить файл для Telegram")
	}
	return s.send(ctx, "sendDocument", mw.FormDataContentType(), body.Bytes())
}
