package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type TGUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Кастомный парсер: работает ТОЧНО как decodeURIComponent в браузере.
// Не превращает знаки '+' в пробелы, в отличие от стандартного url.ParseQuery.
func parseTelegramQuery(query string) map[string]string {
	m := make(map[string]string)
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, err := url.PathUnescape(parts[0])
		if err != nil {
			key = parts[0]
		}

		val, err := url.PathUnescape(parts[1])
		if err != nil {
			val = parts[1]
		}

		m[key] = val
	}
	return m
}

func ValidateInitData(initDataRaw, botToken string) (*TGUser, error) {
	// 1. Очистка токена от мусора Docker
	cleanToken := strings.TrimSpace(botToken)
	cleanToken = strings.Trim(cleanToken, "\"'")
	cleanToken = strings.ReplaceAll(cleanToken, "\r", "")
	cleanToken = strings.ReplaceAll(cleanToken, "\n", "")

	// 2. Используем наш безопасный парсер
	values := parseTelegramQuery(initDataRaw)

	hash, ok := values["hash"]
	if !ok || hash == "" {
		return nil, errors.New("hash missing from initData")
	}

	// 3. Удаляем мусор
	delete(values, "hash")
	delete(values, "signature") // Защита от нового API Телеграма

	// 4. Сортируем
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 5. Собираем строку
	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, values[k]))
	}
	dataCheckString := strings.Join(dataCheckArr, "\n")

	// 6. Хэшируем
	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(cleanToken))
	secretKey := secretMac.Sum(nil)

	dataMac := hmac.New(sha256.New, secretKey)
	dataMac.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(dataMac.Sum(nil))

	// 7. Проверяем
	if calculatedHash != hash {
		return nil, errors.New("invalid hash signature")
	}

	// 8. Читаем юзера
	userStr := values["user"]
	var user TGUser
	if err := json.Unmarshal([]byte(userStr), &user); err != nil {
		return nil, errors.New("failed to parse user json")
	}

	return &user, nil
}
