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

func ValidateInitData(initDataRaw, botToken string) (*TGUser, error) {
	// 1. Очищаем токен от случайных пробелов, переносов строк и кавычек
	// (Частейшая проблема при деплое через Docker и .env файлы)
	cleanToken := strings.TrimSpace(botToken)
	cleanToken = strings.Trim(cleanToken, "\"'")

	// 2. Парсим строку. url.ParseQuery АВТОМАТИЧЕСКИ декодирует значения,
	// что является обязательным требованием Telegram!
	values, err := url.ParseQuery(initDataRaw)
	if err != nil {
		return nil, errors.New("invalid initData format")
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, errors.New("hash missing from initData")
	}

	// 3. УДАЛЯЕМ поля, которые не участвуют в генерации подписи
	values.Del("hash")
	// КРИТИЧНЫЙ ФИКС: Удаляем signature (появилась в новых версиях Telegram)
	values.Del("signature")

	// 4. Сортируем ключи по алфавиту
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 5. Собираем data_check_string
	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, values.Get(k)))
	}
	dataCheckString := strings.Join(dataCheckArr, "\n")

	// 6. Secret key = HMAC_SHA256("WebAppData", botToken)
	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(cleanToken))
	secretKey := secretMac.Sum(nil)

	// 7. Calculated Hash = HMAC_SHA256(dataCheckString, secretKey)
	dataMac := hmac.New(sha256.New, secretKey)
	dataMac.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(dataMac.Sum(nil))

	// 8. Сравниваем подписи
	if calculatedHash != hash {
		return nil, errors.New("invalid hash signature")
	}

	// 9. Парсим пользователя
	userStr := values.Get("user")
	if userStr == "" {
		return nil, errors.New("user field missing in initData")
	}

	var user TGUser
	if err := json.Unmarshal([]byte(userStr), &user); err != nil {
		return nil, errors.New("failed to parse user json")
	}

	return &user, nil
}
