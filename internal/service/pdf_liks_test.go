package service

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestPDFLinks(t *testing.T) {
	s := &PDFService{linkKey: []byte("test-key")}

	createdAt := time.Now()
	tok := s.SignLink(42, createdAt, time.Minute)
	if id, created, err := s.VerifyLink(tok); err != nil || id != 42 || created != createdAt.UnixMicro() {
		t.Fatalf("валидная ссылка: id=%d created=%d err=%v", id, created, err)
	}

	if _, _, err := s.VerifyLink(s.SignLink(42, createdAt, -time.Minute)); err == nil {
		t.Error("просроченная ссылка должна отклоняться")
	}

	if _, _, err := (&PDFService{linkKey: []byte("other")}).VerifyLink(tok); err == nil {
		t.Error("ссылка с чужим ключом должна отклоняться")
	}

	// подделка: меняем номер заказа, подпись оставляем прежней
	body, sig, _ := strings.Cut(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(body)
	forged := base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), "42", "43", 1))) + "." + sig
	if _, _, err := s.VerifyLink(forged); err == nil {
		t.Error("подделанная ссылка должна отклоняться")
	}

	for _, bad := range []string{"", "abc", "a.b", ".", "..."} {
		if _, _, err := s.VerifyLink(bad); err == nil {
			t.Errorf("мусор %q должен отклоняться", bad)
		}
	}

	legacyBody := base64.RawURLEncoding.EncodeToString([]byte(`{"o":42,"e":4102444800}`))
	legacy := legacyBody + "." + base64.RawURLEncoding.EncodeToString(s.mac(legacyBody))
	if _, _, err := s.VerifyLink(legacy); err == nil {
		t.Error("ссылка без времени создания заказа должна отклоняться")
	}
}
