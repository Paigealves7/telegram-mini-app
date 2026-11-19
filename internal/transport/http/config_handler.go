package http

import (
	"net/http"
	"os"
	"strings"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
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
func ConfigHandler(repo *repository.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
			return
		}
		options, err := availableOrganizations(r.Context(), repo, user)
		if err != nil {
			serverError(w, r, "не удалось получить список организаций", err)
			return
		}
		org, _, _ := selectOrganization(r, options, user.IsSuperAdmin)
		role, orgID := domain.Role(user.Role), int64(0)
		if org != nil {
			role, orgID = org.Role, org.ID
		} else if !user.IsSuperAdmin && len(options) == 0 {
			role = domain.RolePending
		}
		pdfWriteJSON(w, http.StatusOK, map[string]any{
			"pdf_send": pdfSendEnabled(), "user_id": user.UserID, "role": role,
			"is_super_admin": user.IsSuperAdmin, "organization_id": orgID,
			"organization": org, "organizations": options,
		})
	}
}
