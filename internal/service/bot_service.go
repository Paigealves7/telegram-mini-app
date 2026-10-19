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
	"strings"
	"telegram-mini-app/internal/repository"
	"time"
)

type BotService struct {
	botToken string
	repo     *repository.Repository
}

func NewBotService(token string, repo *repository.Repository) *BotService {
	return &BotService{botToken: token, repo: repo}
}

func (s *BotService) NotifyUser(ctx context.Context, userID int64, message string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err // Пользователь не найден
	}

	// Наш юзер авторизовался через Telegram, если его логин начинается на "tg_"
	if !strings.HasPrefix(user.Username, "tg_") {
		return nil // Это WEB-юзер (например, админ по логину/паролю), ему не отправить пуш
	}

	// Извлекаем chat_id из логина tg_123456
	chatID := strings.TrimPrefix(user.Username, "tg_")

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.botToken)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

var (
	ErrBotCannotWrite    = errors.New("бот не может написать вам: откройте чат с ботом и нажмите «Запустить» (Start), затем повторите")
	ErrNoTelegramAccount = errors.New("отправка в Telegram доступна только при входе через Telegram")
)

// SendDocument отправляет файл пользователю в личный чат с ботом.
// Тексты ошибок намеренно без URL: в нём лежит токен бота.
func (s *BotService) SendDocument(ctx context.Context, userID int64, filename string, data []byte, caption string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(user.Username, "tg_") {
		return ErrNoTelegramAccount
	}
	chatID := strings.TrimPrefix(user.Username, "tg_")

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+s.botToken+"/sendDocument", &body)
	if err != nil {
		return errors.New("не удалось подготовить запрос к Telegram")
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Telegram недоступен с сервера, попробуйте позже")
	}
	defer resp.Body.Close()

	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out)
	if out.OK {
		return nil
	}

	d := strings.ToLower(out.Description)
	if resp.StatusCode == http.StatusForbidden || strings.Contains(d, "chat not found") || strings.Contains(d, "can't initiate") {
		return ErrBotCannotWrite
	}
	log.Printf("BOT sendDocument: HTTP %d: %s", resp.StatusCode, out.Description)
	return errors.New("Telegram не принял файл")
}
