package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type TGUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// InitData — проверенные данные из Telegram.WebApp.initData.
type InitData struct {
	User     TGUser
	AuthDate time.Time
}

var (
	ErrEmptyToken       = errors.New("telegram: токен бота пустой")
	ErrInvalidFormat    = errors.New("telegram: неверный формат initData")
	ErrInvalidSignature = errors.New("telegram: неверная подпись initData")
	ErrExpired          = errors.New("telegram: initData устарел")
)

const clockSkew = time.Minute

// ValidateInitData проверяет подпись initData и возраст auth_date.
// Всё, что вернулось в InitData, подписано Telegram и ему можно доверять.
//
// Значения полей берутся ДЕКОДИРОВАННЫМИ (url.ParseQuery), как в документации Telegram.
// Поле signature (Ed25519, для сторонних проверяющих) в разных источниках то входит
// в data_check_string, то нет, поэтому проверяем оба варианта. Подделать HMAC
// не получится ни в одном из них.
func ValidateInitData(initDataRaw, botToken string, maxAge time.Duration, now time.Time) (*InitData, error) {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return nil, ErrEmptyToken
	}

	values, err := url.ParseQuery(initDataRaw)
	if err != nil {
		return nil, ErrInvalidFormat
	}

	receivedHash, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(receivedHash) != sha256.Size {
		return nil, ErrInvalidFormat
	}

	secretKey := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	if !signatureMatches(values, secretKey, receivedHash, true) &&
		!signatureMatches(values, secretKey, receivedHash, false) {
		return nil, ErrInvalidSignature
	}

	authUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil || authUnix <= 0 {
		return nil, ErrInvalidFormat
	}
	authDate := time.Unix(authUnix, 0)
	if authDate.After(now.Add(clockSkew)) || now.Sub(authDate) > maxAge {
		return nil, ErrExpired
	}

	var user TGUser
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID == 0 {
		return nil, ErrInvalidFormat
	}

	return &InitData{User: user, AuthDate: authDate}, nil
}

func signatureMatches(values url.Values, secretKey, want []byte, includeSignature bool) bool {
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == "hash" || (k == "signature" && !includeSignature) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + values.Get(k)
	}

	got := hmacSHA256(secretKey, []byte(strings.Join(parts, "\n")))
	return hmac.Equal(got, want)
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}
