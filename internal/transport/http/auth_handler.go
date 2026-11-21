package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	// Сессия после входа по логину и паролю (веб-версия: бухгалтерия и др.).
	passwordSessionTTL = 7 * 24 * time.Hour
	// Сессия после входа через Telegram: Telegram сам подтверждает личность при каждом открытии.
	telegramSessionTTL = 24 * time.Hour
)

type Claims struct {
	OrganizationID   int64  `json:"organization_id,omitempty"`
	OrganizationName string `json:"-"`
	UserID           int64  `json:"user_id"`
	Role             string `json:"role"`
	IsSuperAdmin     bool   `json:"is_super_admin"`
	jwt.RegisteredClaims
}

func (c *Claims) setUser(u *domain.User) {
	c.UserID = u.ID
	c.Role = string(u.Role)
	c.IsSuperAdmin = u.IsSuperAdmin()
}

type AuthHandler struct {
	repo      *repository.Repository
	jwtSecret []byte
}

// Принимаем секрет в конструкторе
func NewAuthHandler(repo *repository.Repository, secret string) *AuthHandler {
	return &AuthHandler{
		repo:      repo,
		jwtSecret: []byte(secret),
	}
}

type RegisterRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     domain.Role `json:"role"`
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

func validWebUsername(w http.ResponseWriter, username string) bool {
	if !usernameRe.MatchString(username) {
		http.Error(w, "логин: 3–32 символа, латиница, цифры, точка, дефис, подчёркивание", http.StatusBadRequest)
		return false
	}
	if strings.HasPrefix(strings.ToLower(username), "tg_") {
		http.Error(w, "логин не может начинаться с tg_", http.StatusBadRequest)
		return false
	}
	return true
}

func hashWebPassword(w http.ResponseWriter, password string) (string, bool) {
	if len(password) < 8 || len(password) > 72 {
		http.Error(w, "пароль: от 8 до 72 байт", http.StatusBadRequest)
		return "", false
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "ошибка хеширования пароля", http.StatusInternalServerError)
		return "", false
	}
	return string(hash), true
}

// Register создаёт сотрудника с паролем.
// Маршрут POST /api/v1/users доступен только менеджеру (см. router.go).
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
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
	case "":
		req.Role = domain.RoleWorker
	case domain.RoleManager, domain.RoleMaster, domain.RoleWorker:
	default:
		http.Error(w, "неизвестная роль", http.StatusBadRequest)
		return
	}
	actor, ok := currentUser(r)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, "выберите организацию", http.StatusConflict)
		return
	}
	if (req.Role == domain.RoleManager || (domain.User{Username: req.Username}).IsSuperAdmin()) &&
		!actor.IsSuperAdmin {
		http.Error(w, "действие доступно только супер-админу", http.StatusForbidden)
		return
	}

	hashedPassword, ok := hashWebPassword(w, req.Password)
	if !ok {
		return
	}

	ctx := r.Context()
	user, err := h.repo.CreateUser(ctx, req.Username, hashedPassword, req.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			serverError(w, r, "не удалось создать пользователя", err)
			return
		}
		user, err = h.repo.GetUserByUsername(ctx, req.Username)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			http.Error(w, "пользователь с таким логином уже существует", http.StatusConflict)
			return
		}
	}
	if err := h.repo.AddOrganizationMember(ctx, actor.OrganizationID, user.ID, req.Role); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "пользователь уже состоит в организации", http.StatusConflict)
			return
		}
		serverError(w, r, "не удалось добавить пользователя в организацию", err)
		return
	}
	user.Role = req.Role

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) RegisterInvite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		LoginRequest
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		http.Error(w, "укажите код приглашения", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !validWebUsername(w, req.Username) {
		return
	}
	hash, ok := hashWebPassword(w, req.Password)
	if !ok {
		return
	}
	user, orgID, err := h.repo.RegisterWithInvite(r.Context(), inviteHash(req.Code), req.Username, hash, func(storedHash string) bool {
		return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)) == nil
	})
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrInviteNotFound):
		http.Error(w, "приглашение не найдено", http.StatusNotFound)
		return
	case errors.Is(err, repository.ErrInviteUnavailable), errors.Is(err, repository.ErrOrganizationNotFound):
		http.Error(w, "приглашение недоступно", http.StatusConflict)
		return
	case errors.Is(err, repository.ErrOrganizationMemberExists):
		http.Error(w, "вы уже состоите в этой лесопилке. Войдите по логину и паролю", http.StatusConflict)
		return
	case errors.Is(err, repository.ErrInviteCredentials):
		http.Error(w, "выберите другой логин или укажите действующий пароль", http.StatusConflict)
		return
	default:
		serverError(w, r, "не удалось принять приглашение", err)
		return
	}
	claims := &Claims{OrganizationID: orgID}
	claims.setUser(user)
	token, err := generateToken(claims, h.jwtSecret, passwordSessionTTL)
	if err != nil {
		serverError(w, r, "ошибка генерации токена", err)
		return
	}
	pdfWriteJSON(w, http.StatusOK, map[string]any{"token": token, "organization_id": orgID})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}

	user, err := h.repo.GetUserByUsername(r.Context(), req.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		http.Error(w, "неверный логин или пароль", http.StatusUnauthorized)
		return
	}

	claims := &Claims{}
	claims.setUser(user)

	tokenString, err := generateToken(claims, h.jwtSecret, passwordSessionTTL)
	if err != nil {
		http.Error(w, "ошибка генерации токена", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenString,
	})
}

// Теперь функция требует передачи ключа
func generateToken(claims *Claims, secret []byte, ttl time.Duration) (string, error) {
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(ttl))
	claims.IssuedAt = jwt.NewNumericDate(time.Now())

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
