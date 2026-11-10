package http

import (
	"net/http"
	"os"
	"strings"
)

// pdfSendEnabled: PDF_SEND_ENABLED=false в .env отключает отправку накладных в чат (например, из-за блокировок).
func pdfSendEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_SEND_ENABLED"))) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

// GET /api/v1/config: настройки, от которых зависит вид приложения (для любого вошедшего пользователя).
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	pdfWriteJSON(w, http.StatusOK, map[string]bool{"pdf_send": pdfSendEnabled()})
}
