package http

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

var warnNoProxyHeader sync.Once

func parseIP(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return ""
	}
	return ip.String()
}

// clientIP определяет настоящий адрес клиента.
// Заголовки X-Real-IP / X-Forwarded-For принимаются ТОЛЬКО если запрос пришёл от прокси
// из частной сети (nginx на этом же сервере или в docker-сети). Запрос напрямую из интернета
// не может подделать адрес заголовком.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil {
		return host
	}
	if peer.IsLoopback() || peer.IsPrivate() {
		if v := parseIP(r.Header.Get("X-Real-IP")); v != "" {
			return v
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// Последний элемент дописал наш прокси; всё левее может прислать сам клиент.
			if v := parseIP(parts[len(parts)-1]); v != "" {
				return v
			}
		}
		warnNoProxyHeader.Do(func() {
			log.Printf("RATE: запрос от прокси без X-Real-IP: лимиты считаются по адресу прокси. Добавьте в nginx: proxy_set_header X-Real-IP $remote_addr;")
		})
	}
	return peer.String()
}
