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
