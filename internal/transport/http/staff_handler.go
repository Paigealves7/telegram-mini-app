package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// StaffHandler: управление сотрудниками. Все маршруты доступны только руководителю (см. router.go).
type StaffHandler struct {
	repo *repository.Repository
}

func NewStaffHandler(repo *repository.Repository) *StaffHandler {
	return &StaffHandler{repo: repo}
}

type staffItem struct {
	ID        int64       `json:"id"`
	Username  string      `json:"username"`
	Role      domain.Role `json:"role"`
	Kind      string      `json:"kind"`        // "telegram" или "password"
	Name      string      `json:"name"`        // имя из Telegram (пусто, пока человек не открыл приложение после обновления)
	TGUser    string      `json:"tg_username"` // @username без «@»
	CreatedAt time.Time   `json:"created_at"`
}

func (h *StaffHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.ListUsers(r.Context())
	if err != nil {
		serverError(w, r, "не удалось получить список сотрудников", err)
		return
	}
	out := make([]staffItem, 0, len(users))
	for _, u := range users {
		kind := "password"
		if strings.HasPrefix(u.PasswordHash, tgPasswordPrefix) {
			kind = "telegram"
		}
		out = append(out, staffItem{ID: u.ID, Username: u.Username, Role: u.Role, Kind: kind, Name: u.DisplayName, TGUser: u.TGUsername, CreatedAt: u.CreatedAt})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func staffID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "некорректный номер сотрудника", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// SetRole: менеджер может назначить роль мастера или сотрудника. Назначить или изменить роль руководителя нельзя.
func (h *StaffHandler) SetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := staffID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Role domain.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.Role != domain.RoleMaster && req.Role != domain.RoleWorker {
		http.Error(w, "можно назначить только роль мастера или сотрудника", http.StatusBadRequest)
		return
	}
	updated, err := h.repo.UpdateUserRole(r.Context(), id, req.Role)
	if err != nil {
		serverError(w, r, "не удалось изменить роль", err)
		return
	}
	if !updated {
		http.Error(w, "сотрудник не найден или это руководитель (его роль не меняется)", http.StatusNotFound)
		return
	}
	if claims, ok := r.Context().Value(UserContextKey).(*Claims); ok {
		log.Printf("STAFF: пользователь %d сменил роль сотрудника %d на %s", claims.UserID, id, req.Role)
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetPassword: новый пароль для аккаунта с паролем. У Telegram-аккаунтов пароля нет.
func (h *StaffHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := staffID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 72 {
		http.Error(w, "пароль: от 8 до 72 символов", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "ошибка хеширования пароля", http.StatusInternalServerError)
		return
	}
	updated, err := h.repo.UpdateUserPassword(r.Context(), id, string(hash))
	if err != nil {
		serverError(w, r, "не удалось изменить пароль", err)
		return
	}
	if !updated {
		http.Error(w, "сотрудник не найден или входит через Telegram (у него нет пароля)", http.StatusNotFound)
		return
	}
	if claims, ok := r.Context().Value(UserContextKey).(*Claims); ok {
		log.Printf("STAFF: пользователь %d сменил пароль сотрудника %d", claims.UserID, id)
	}
	w.WriteHeader(http.StatusNoContent)
}
