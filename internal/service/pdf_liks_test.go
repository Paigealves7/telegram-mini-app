package service

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestPDFLinks(t *testing.T) {
	s := &PDFService{linkKey: []byte("test-key")}

	tok := s.SignLink(42, time.Minute)
	if id, err := s.VerifyLink(tok); err != nil || id != 42 {
		t.Fatalf("валидная ссылка: id=%d err=%v", id, err)
	}

	if _, err := s.VerifyLink(s.SignLink(42, -time.Minute)); err == nil {
		t.Error("просроченная ссылка должна отклоняться")
	}

	if _, err := (&PDFService{linkKey: []byte("other")}).VerifyLink(tok); err == nil {
		t.Error("ссылка с чужим ключом должна отклоняться")
	}

	// подделка: меняем номер заказа, подпись оставляем прежней
	body, sig, _ := strings.Cut(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(body)
	forged := base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), "42", "43", 1))) + "." + sig
	if _, err := s.VerifyLink(forged); err == nil {
		t.Error("подделанная ссылка должна отклоняться")
	}

	for _, bad := range []string{"", "abc", "a.b", ".", "..."} {
		if _, err := s.VerifyLink(bad); err == nil {
			t.Errorf("мусор %q должен отклоняться", bad)
		}
	}
}
