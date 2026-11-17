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
	out := map[string]any{"pdf_send": pdfSendEnabled()}
	if user, ok := currentUser(r); ok {
		out["user_id"], out["role"], out["is_super_admin"] = user.UserID, user.Role, user.IsSuperAdmin
	}
	pdfWriteJSON(w, http.StatusOK, out)
}
