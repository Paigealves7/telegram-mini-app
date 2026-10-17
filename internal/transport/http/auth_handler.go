package http

import (
	"encoding/json"
	"errors"
	"log"
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

type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
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
	if !usernameRe.MatchString(req.Username) {
		http.Error(w, "логин: 3-32 символа, латиница, цифры, точка, дефис, подчёркивание", http.StatusBadRequest)
		return
	}
	// Префикс tg_ зарезервирован под аккаунты, созданные через Telegram.
	if strings.HasPrefix(strings.ToLower(req.Username), "tg_") {
		http.Error(w, "логин не может начинаться с tg_", http.StatusBadRequest)
		return
	}
	// У bcrypt предел 72 байта.
	if len(req.Password) < 8 || len(req.Password) > 72 {
		http.Error(w, "пароль: от 8 до 72 символов", http.StatusBadRequest)
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

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "ошибка хеширования пароля", http.StatusInternalServerError)
		return
	}

	user, err := h.repo.CreateUser(r.Context(), req.Username, string(hashedPassword), req.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "пользователь с таким логином уже существует", http.StatusConflict)
			return
		}
		log.Printf("REGISTER: не удалось создать пользователя %q: %v", req.Username, err)
		http.Error(w, "не удалось создать пользователя", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

	claims := &Claims{
		UserID: user.ID,
		Role:   string(user.Role),
	}

	tokenString, err := generateToken(claims, h.jwtSecret)
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
func generateToken(claims *Claims, secret []byte) (string, error) {
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(24 * time.Hour))
	claims.IssuedAt = jwt.NewNumericDate(time.Now())

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
