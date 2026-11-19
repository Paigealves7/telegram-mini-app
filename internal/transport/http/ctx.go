package http

import "net/http"

// currentUser возвращает данные пользователя из проверенного JWT (его кладёт AuthMiddleware).
// Только отсюда берём «кто выполнил операцию»: значения из тела запроса клиенту доверять нельзя.
func currentUser(r *http.Request) (*Claims, bool) {
	c, ok := r.Context().Value(UserContextKey).(*Claims)
	if !ok || c == nil || c.UserID <= 0 {
		return nil, false
	}
	return c, true
}

func currentOrgID(r *http.Request) (int64, bool) {
	c, ok := currentUser(r)
	if !ok {
		return 0, false
	}
	return c.OrganizationID, c.OrganizationID > 0
}

func requireOrgID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := currentOrgID(r)
	if !ok {
		http.Error(w, "выберите организацию", http.StatusConflict)
	}
	return id, ok
}
