package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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
			claims.OrganizationID, claims.OrganizationName = 0, ""
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

type organizationOption struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	CompanyInfo string      `json:"company_info"`
	IsDemo      bool        `json:"is_demo"`
	IsActive    bool        `json:"is_active"`
	Role        domain.Role `json:"role"`
}

func availableOrganizations(ctx context.Context, repo *repository.Repository, claims *Claims) ([]organizationOption, error) {
	memberships, err := repo.ListUserOrganizations(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	options := make([]organizationOption, 0, len(memberships))
	if !claims.IsSuperAdmin {
		for _, m := range memberships {
			options = append(options, organizationOption{m.OrganizationID, m.Name, m.CompanyInfo, m.IsDemo, m.IsActive, m.Role})
		}
		return options, nil
	}
	orgs, err := repo.ListOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	roles := make(map[int64]domain.Role, len(memberships))
	for _, m := range memberships {
		roles[m.OrganizationID] = m.Role
	}
	for _, org := range orgs {
		role := roles[org.ID]
		if role == "" {
			role = domain.RoleManager
		}
		options = append(options, organizationOption{org.ID, org.Name, org.CompanyInfo, org.IsDemo, org.IsActive, role})
	}
	return options, nil
}

func selectOrganization(r *http.Request, options []organizationOption, isSuperAdmin bool) (*organizationOption, int, string) {
	if values := r.Header.Values("X-Organization-ID"); len(values) > 0 {
		id, err := strconv.ParseInt(strings.TrimSpace(values[0]), 10, 64)
		if len(values) != 1 || err != nil || id <= 0 {
			return nil, http.StatusBadRequest, "некорректный идентификатор организации"
		}
		for _, org := range options {
			if org.ID == id {
				return &org, 0, ""
			}
		}
		if isSuperAdmin {
			return nil, http.StatusNotFound, "организация не найдена"
		}
		return nil, http.StatusForbidden, "нет доступа к выбранной организации"
	}
	if len(options) == 1 && options[0].IsActive {
		return &options[0], 0, ""
	}
	if len(options) == 0 && !isSuperAdmin {
		return nil, http.StatusForbidden, "доступ ожидает приглашения в организацию"
	}
	return nil, http.StatusConflict, "выберите организацию"
}

func ResolveOrganization(repo *repository.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := currentUser(r)
			if !ok {
				http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
				return
			}
			options, err := availableOrganizations(r.Context(), repo, claims)
			if err != nil {
				serverError(w, r, "не удалось получить список организаций", err)
				return
			}
			org, status, message := selectOrganization(r, options, claims.IsSuperAdmin)
			if org == nil {
				http.Error(w, message, status)
				return
			}
			if claims.IsSuperAdmin && len(r.Header.Values("X-Organization-ID")) > 0 {
				organization, err := repo.GetOrganizationByID(r.Context(), org.ID)
				if errors.Is(err, repository.ErrOrganizationNotFound) {
					http.Error(w, "организация не найдена", http.StatusNotFound)
					return
				}
				if err != nil {
					serverError(w, r, "не удалось получить организацию", err)
					return
				}
				org.Name = organization.Name
			}
			scoped := *claims
			scoped.OrganizationID, scoped.OrganizationName, scoped.Role = org.ID, org.Name, string(org.Role)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserContextKey, &scoped)))
		})
	}
}
