package telegram

import (
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testToken = "123456:TEST-token"

func buildInitData(token string, fields map[string]string, hashCoversSignature bool) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k == "signature" && !hashCoversSignature {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + fields[k]
	}

	secret := hmacSHA256([]byte("WebAppData"), []byte(token))
	hash := hex.EncodeToString(hmacSHA256(secret, []byte(strings.Join(parts, "\n"))))

	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hash)
	return q.Encode()
}

func baseFields(authDate time.Time) map[string]string {
	return map[string]string{
		"auth_date": strconv.FormatInt(authDate.Unix(), 10),
		"query_id":  "AAH-test",
		"user":      `{"id":42,"first_name":"Иван","username":"ivan"}`,
	}
}

func TestValidateInitData(t *testing.T) {
	now := time.Now()
	maxAge := 24 * time.Hour

	withSig := baseFields(now)
	withSig["signature"] = "abc_DEF-123"

	tampered, _ := url.ParseQuery(buildInitData(testToken, baseFields(now), true))
	tampered.Set("user", `{"id":1,"first_name":"Чужой"}`)

	noHash, _ := url.ParseQuery(buildInitData(testToken, baseFields(now), true))
	noHash.Del("hash")

	tests := []struct {
		name    string
		raw     string
		token   string
		wantErr error
	}{
		{"валидные данные", buildInitData(testToken, baseFields(now), true), testToken, nil},
		{"токен с пробелами и переводом строки", buildInitData(testToken, baseFields(now), true), "  " + testToken + "\n", nil},
		{"подпись покрывает signature", buildInitData(testToken, withSig, true), testToken, nil},
		{"подпись не покрывает signature", buildInitData(testToken, withSig, false), testToken, nil},
		{"чужой токен", buildInitData(testToken, baseFields(now), true), "999:OTHER", ErrInvalidSignature},
		{"подменён user", tampered.Encode(), testToken, ErrInvalidSignature},
		{"нет hash", noHash.Encode(), testToken, ErrInvalidFormat},
		{"устарел", buildInitData(testToken, baseFields(now.Add(-48*time.Hour)), true), testToken, ErrExpired},
		{"дата из будущего", buildInitData(testToken, baseFields(now.Add(2*time.Hour)), true), testToken, ErrExpired},
		{"пустой токен", buildInitData(testToken, baseFields(now), true), "", ErrEmptyToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateInitData(tt.raw, tt.token, maxAge, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ожидалась ошибка %v, получена %v", tt.wantErr, err)
			}
			if tt.wantErr == nil && got.User.ID != 42 {
				t.Fatalf("ожидался user id 42, получен %d", got.User.ID)
			}
		})
	}
}
