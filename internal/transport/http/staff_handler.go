package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// StaffHandler: управление сотрудниками. Все маршруты доступны только руководителю (см. router.go).
type StaffHandler struct {
	repo *repository.Repository
}

func NewStaffHandler(repo *repository.Repository) *StaffHandler {
	return &StaffHandler{repo: repo}
}

type staffItem struct {
	ID           int64       `json:"id"`
	Username     string      `json:"username"`
	Role         domain.Role `json:"role"`
	Kind         string      `json:"kind"`        // "telegram" или "password"
	Name         string      `json:"name"`        // имя из Telegram (пусто, пока человек не открыл приложение после обновления)
	TGUser       string      `json:"tg_username"` // @username без «@»
	CreatedAt    time.Time   `json:"created_at"`
	IsSuperAdmin bool        `json:"is_super_admin"`
}

func (h *StaffHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return
	}
	h.list(w, r, orgID)
}

func (h *StaffHandler) ListAdminMembers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := adminID(w, r)
	if !ok {
		return
	}
	if _, err := h.repo.GetOrganizationByID(r.Context(), orgID); err != nil {
		adminResult(w, r, "не удалось получить организацию", err)
		return
	}
	h.list(w, r, orgID)
}

func (h *StaffHandler) list(w http.ResponseWriter, r *http.Request, orgID int64) {
	users, err := h.repo.ListUsers(r.Context(), orgID)
	if err != nil {
		serverError(w, r, "не удалось получить список сотрудников", err)
		return
	}
	out := make([]staffItem, 0, len(users))
	for _, u := range users {
		out = append(out, newStaffItem(&u))
	}
	pdfWriteJSON(w, http.StatusOK, out)
}

func newStaffItem(u *domain.User) staffItem {
	kind := "password"
	if strings.HasPrefix(u.PasswordHash, tgPasswordPrefix) {
		kind = "telegram"
	}
	return staffItem{ID: u.ID, Username: u.Username, Role: u.Role, Kind: kind, Name: u.DisplayName, TGUser: u.TGUsername, CreatedAt: u.CreatedAt, IsSuperAdmin: u.IsSuperAdmin()}
}

func (h *StaffHandler) CreateAdminWebUser(w http.ResponseWriter, r *http.Request) {
	orgID, ok := adminID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !validWebUsername(w, req.Username) {
		return
	}
	switch req.Role {
	case domain.RoleManager, domain.RoleMaster, domain.RoleWorker:
	default:
		http.Error(w, "неизвестная роль", http.StatusBadRequest)
		return
	}
	var hash string
	if req.Password != "" {
		hash, ok = hashWebPassword(w, req.Password)
		if !ok {
			return
		}
	}
	user, err := h.repo.CreateOrganizationWebUser(r.Context(), orgID, req.Username, hash, req.Role)
	switch {
	case err == nil:
		pdfWriteJSON(w, http.StatusCreated, newStaffItem(user))
	case errors.Is(err, repository.ErrWebUserNotFound):
		http.Error(w, "для нового аккаунта укажите пароль", http.StatusBadRequest)
	case errors.Is(err, repository.ErrWebUserProtected):
		http.Error(w, "этот аккаунт нельзя добавить через веб-доступ", http.StatusConflict)
	case errors.Is(err, repository.ErrOrganizationMemberExists):
		http.Error(w, "пользователь уже состоит в лесопилке", http.StatusConflict)
	default:
		adminResult(w, r, "не удалось создать веб-аккаунт", err)
	}
}

func (h *StaffHandler) SetAdminWebPassword(w http.ResponseWriter, r *http.Request) {
	orgID, ok := adminID(w, r)
	if !ok {
		return
	}
	userID, ok := recordID(w, r, "userID")
	if !ok {
		return
	}
	hash, ok := readWebPassword(w, r)
	if !ok {
		return
	}
	updated, err := h.repo.UpdateOrganizationUserPassword(r.Context(), orgID, userID, hash)
	switch {
	case errors.Is(err, repository.ErrWebUserProtected):
		http.Error(w, "нельзя изменить пароль этого аккаунта", http.StatusConflict)
	case err != nil:
		adminResult(w, r, "не удалось изменить пароль", err)
	case !updated:
		http.Error(w, "веб-аккаунт сотрудника не найден", http.StatusNotFound)
	default:
		actor, _ := currentUser(r)
		log.Printf("STAFF: пользователь %d сменил пароль сотрудника %d", actor.UserID, userID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *StaffHandler) DeleteAdminMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := adminID(w, r)
	if !ok {
		return
	}
	userID, ok := recordID(w, r, "userID")
	if !ok {
		return
	}
	actor, _ := currentUser(r)
	if actor.UserID == userID {
		http.Error(w, "нельзя удалить собственное членство", http.StatusConflict)
		return
	}
	err := h.repo.DeleteOrganizationMember(r.Context(), orgID, userID)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, repository.ErrWebUserProtected):
		http.Error(w, "нельзя удалить членство супер-администратора", http.StatusConflict)
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, repository.ErrOrganizationMembershipNotFound):
		http.Error(w, "сотрудник не найден в лесопилке", http.StatusNotFound)
	default:
		adminResult(w, r, "не удалось удалить сотрудника", err)
	}
}

func staffID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "некорректный номер сотрудника", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (h *StaffHandler) SetRole(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return
	}
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
	if req.Role != domain.RoleManager && req.Role != domain.RoleMaster && req.Role != domain.RoleWorker {
		http.Error(w, "неизвестная роль", http.StatusBadRequest)
		return
	}
	actor, _ := currentUser(r)
	if actor.UserID == id {
		http.Error(w, "нельзя изменить собственную роль", http.StatusForbidden)
		return
	}
	target := h.staffTarget(w, r, id)
	if target == nil {
		return
	}
	if !actor.IsSuperAdmin && (req.Role == domain.RoleManager || target.Role == domain.RoleManager) {
		http.Error(w, "управление ролями руководителей доступно только супер-админу", http.StatusForbidden)
		return
	}
	updated, err := h.repo.UpdateUserRole(r.Context(), orgID, id, req.Role, actor.IsSuperAdmin)
	if err != nil {
		serverError(w, r, "не удалось изменить роль", err)
		return
	}
	if !updated {
		http.Error(w, "роль не изменена: данные сотрудника обновились", http.StatusConflict)
		return
	}
	if claims, ok := r.Context().Value(UserContextKey).(*Claims); ok {
		log.Printf("STAFF: пользователь %d сменил роль сотрудника %d на %s", claims.UserID, id, req.Role)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *StaffHandler) staffTarget(w http.ResponseWriter, r *http.Request, id int64) *domain.User {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return nil
	}
	role, err := h.repo.GetUserOrganizationRole(r.Context(), orgID, id)
	if errors.Is(err, repository.ErrOrganizationMembershipNotFound) {
		http.Error(w, "сотрудник не найден", http.StatusNotFound)
		return nil
	}
	if err != nil {
		serverError(w, r, "не удалось проверить членство сотрудника", err)
		return nil
	}
	target, err := h.repo.GetUserByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "сотрудник не найден", http.StatusNotFound)
		return nil
	}
	if err != nil {
		serverError(w, r, "не удалось получить данные сотрудника", err)
		return nil
	}
	actor, _ := currentUser(r)
	if target.IsSuperAdmin() && !actor.IsSuperAdmin {
		http.Error(w, "управление аккаунтом супер-админа запрещено", http.StatusForbidden)
		return nil
	}
	target.Role = role
	return target
}

func (h *StaffHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return
	}
	id, ok := staffID(w, r)
	if !ok {
		return
	}
	actor, _ := currentUser(r)
	if actor.UserID == id {
		http.Error(w, "нельзя удалить собственный аккаунт", http.StatusForbidden)
		return
	}
	adminResult(w, r, "не удалось удалить сотрудника", h.repo.DeleteUser(r.Context(), orgID, id))
}

// SetPassword: новый пароль для аккаунта с паролем. У Telegram-аккаунтов пароля нет.
func (h *StaffHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := currentUser(r)
	if !ok || !actor.IsSuperAdmin {
		http.Error(w, "смена пароля аккаунта доступна только супер-админу", http.StatusForbidden)
		return
	}
	id, ok := staffID(w, r)
	if !ok {
		return
	}
	if h.staffTarget(w, r, id) == nil {
		return
	}
	hash, ok := readWebPassword(w, r)
	if !ok {
		return
	}
	updated, err := h.repo.UpdateUserPassword(r.Context(), id, hash)
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

func readWebPassword(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return "", false
	}
	return hashWebPassword(w, req.Password)
}
