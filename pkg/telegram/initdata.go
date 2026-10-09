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
        "log"
)

type TGUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// ValidateInitData проверяет HMAC подпись initData строки от Telegram
func ValidateInitData(initDataRaw, botToken string) (*TGUser, error) {
	values, err := url.ParseQuery(initDataRaw)
	if err != nil {
		return nil, errors.New("invalid initData format")
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, errors.New("hash missing from initData")
	}

	// Удаляем hash из пар ключей для построения data_check_string
	values.Del("hash")

	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, values.Get(k)))
	}
	dataCheckString := strings.Join(dataCheckArr, "\n")

        // 1. Secret key = HMAC_SHA256(botToken, "WebAppData")
        secretMac := hmac.New(sha256.New, []byte(botToken))
        secretMac.Write([]byte("WebAppData"))
        secretKey := secretMac.Sum(nil)

	// 2. Calculated Hash = HMAC_SHA256(dataCheckString, secretKey)
	dataMac := hmac.New(sha256.New, secretKey)
	dataMac.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(dataMac.Sum(nil))

        if calculatedHash != hash {
            log.Printf("DEBUG dataCheckString: %q", dataCheckString)
            log.Printf("DEBUG botToken: %q", botToken)
            log.Printf("DEBUG calculatedHash: %s", calculatedHash)
            log.Printf("DEBUG receivedHash: %s", hash)
            return nil, errors.New("invalid hash signature")
        }        

	// Извлекаем объект user из initData
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
