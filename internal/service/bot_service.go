package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"telegram-mini-app/internal/repository"
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
