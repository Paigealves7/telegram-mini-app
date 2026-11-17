package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

type contextKey string

const UserContextKey contextKey = "user_claims"

// Теперь AuthMiddleware это функция-фабрика, которая замыкает в себе секрет
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	jwtSecret := []byte(secret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "отсутствует заголовок Authorization", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "неверный формат токена", http.StatusUnauthorized)
				return
			}

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (interface{}, error) {
				return jwtSecret, nil
			}, jwt.WithValidMethods([]string{"HS256"})) // другие алгоритмы подписи не принимаем

			if err != nil || !token.Valid {
				http.Error(w, "невалидный или просроченный токен", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RefreshUser применяет изменения прав и удаление аккаунта к уже выданным токенам.
func RefreshUser(repo *repository.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := currentUser(r)
			if !ok {
				http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
				return
			}
			user, err := repo.GetUserByID(r.Context(), claims.UserID)
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "аккаунт удалён, войдите снова", http.StatusUnauthorized)
				return
			}
			if err != nil {
				serverError(w, r, "не удалось проверить права пользователя", err)
				return
			}
			claims.setUser(user)
			next.ServeHTTP(w, r)
		})
	}
}

func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := currentUser(r)
		if !ok || !claims.IsSuperAdmin {
			http.Error(w, "действие доступно только супер-админу", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireRoles(allowedRoles ...domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(UserContextKey).(*Claims)
			if !ok {
				http.Error(w, "ошибка авторизации: данные пользователя не найдены", http.StatusUnauthorized)
				return
			}

			userRole := domain.Role(claims.Role)
			if userRole == domain.RolePending {
				http.Error(w, "доступ ожидает подтверждения руководителем", http.StatusForbidden)
				return
			}
			isAllowed := claims.IsSuperAdmin

			for _, role := range allowedRoles {
				if userRole == role {
					isAllowed = true
					break
				}
			}

			if !isAllowed {
				http.Error(w, "доступ запрещен: недостаточно прав", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
