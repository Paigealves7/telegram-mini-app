package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	cleanToken := strings.TrimSpace(botToken)
	cleanToken = strings.Trim(cleanToken, "\"'")
	// Удаляем возможные Windows-переносы, которые Docker иногда тянет из .env
	cleanToken = strings.ReplaceAll(cleanToken, "\r", "")
	cleanToken = strings.ReplaceAll(cleanToken, "\n", "")

	safePrefix := "---"
	if len(cleanToken) > 5 {
		safePrefix = cleanToken[:5]
	}

	log.Printf("=== TELEGRAM VALIDATION DEBUG ===")
	log.Printf("1. Token length: %d (Starts with: %s...)", len(cleanToken), safePrefix)

	values, err := url.ParseQuery(initDataRaw)
	if err != nil {
		return nil, errors.New("invalid initData format")
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, errors.New("hash missing from initData")
	}

	values.Del("hash")
	values.Del("signature")

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

	log.Printf("2. DataCheckString:\n%s", dataCheckString)

	secretMac := hmac.New(sha256.New, []byte("WebAppData"))
	secretMac.Write([]byte(cleanToken))
	secretKey := secretMac.Sum(nil)

	dataMac := hmac.New(sha256.New, secretKey)
	dataMac.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(dataMac.Sum(nil))

	log.Printf("3. Expected Hash: %s", hash)
	log.Printf("4. Calc'd Hash:   %s", calculatedHash)
	log.Printf("=================================")

	if calculatedHash != hash {
		return nil, errors.New("invalid hash signature")
	}

	userStr := values.Get("user")
	var user TGUser
	if err := json.Unmarshal([]byte(userStr), &user); err != nil {
		return nil, errors.New("failed to parse user json")
	}

	return &user, nil
}
