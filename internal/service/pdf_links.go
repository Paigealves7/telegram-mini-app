package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Короткоживущие подписанные ссылки на PDF.
// Нужны потому, что клиенты Telegram (downloadFile) и внешний браузер не могут передать
// заголовок Authorization. В ссылке зашит номер заказа и срок действия, подпись — HMAC-SHA256.

var ErrBadLink = errors.New("ссылка недействительна или устарела")

type linkPayload struct {
	G int64 `json:"g"` // идентификатор организации
	O int64 `json:"o"` // номер заказа
	E int64 `json:"e"` // срок действия, unix-секунды
	C int64 `json:"c"` // время создания конкретного заказа, unix-микросекунды
}

func (s *PDFService) mac(body string) []byte {
	m := hmac.New(sha256.New, s.linkKey)
	m.Write([]byte(body))
	return m.Sum(nil)
}

// SignLink выдаёт токен для заказа на время ttl.
func (s *PDFService) SignLink(orgID, orderID int64, createdAt time.Time, ttl time.Duration) string {
	p, _ := json.Marshal(linkPayload{G: orgID, O: orderID, E: time.Now().Add(ttl).Unix(), C: createdAt.UnixMicro()})
	body := base64.RawURLEncoding.EncodeToString(p)
	return body + "." + base64.RawURLEncoding.EncodeToString(s.mac(body))
}

// VerifyLink проверяет подпись и срок, возвращает номер заказа.
func (s *PDFService) VerifyLink(token string) (orgID int64, orderID int64, createdAt int64, err error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return 0, 0, 0, ErrBadLink
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(want, s.mac(body)) {
		return 0, 0, 0, ErrBadLink
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return 0, 0, 0, ErrBadLink
	}
	var p linkPayload
	if json.Unmarshal(raw, &p) != nil || p.G <= 0 || p.O <= 0 || p.C == 0 || time.Now().Unix() > p.E {
		return 0, 0, 0, ErrBadLink
	}
	return p.G, p.O, p.C, nil
}
