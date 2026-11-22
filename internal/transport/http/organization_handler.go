package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type OrganizationHandler struct {
	repo *repository.Repository
}

type inviteRequest struct {
	Role          domain.Role `json:"role"`
	MaxUses       *int        `json:"max_uses"`
	ExpiresInDays *int        `json:"expires_in_days"`
}

const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func inviteHash(code string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

func (h OrganizationHandler) issueInvite(ctx context.Context, orgID, actorID int64, role domain.Role, maxUses, days int) (*domain.OrganizationInvite, error) {
	var code [10]byte
	if _, err := rand.Read(code[:]); err != nil {
		return nil, err
	}
	for i, b := range code {
		code[i] = inviteAlphabet[int(b)%len(inviteAlphabet)]
	}
	plainCode := string(code[:])
	return h.repo.CreateInvite(ctx, orgID, inviteHash(plainCode), role, maxUses, time.Now().AddDate(0, 0, days), actorID, plainCode)
}

func (h OrganizationHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		http.Error(w, "укажите код приглашения", http.StatusBadRequest)
		return
	}
	org, err := h.repo.AcceptInvite(r.Context(), inviteHash(req.Code), user.UserID)
	switch {
	case err == nil:
		pdfWriteJSON(w, http.StatusOK, org)
	case errors.Is(err, repository.ErrInviteNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, repository.ErrDeletedUser), errors.Is(err, repository.ErrOrganizationNotFound):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, repository.ErrInviteUnavailable):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		serverError(w, r, "не удалось принять приглашение", err)
	}
}

func (h OrganizationHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	if orgID, ok := requireOrgID(w, r); ok {
		actor, _ := currentUser(r)
		h.listInvites(w, r, orgID, actor.IsSuperAdmin)
	}
}

func (h OrganizationHandler) ListAdminInvites(w http.ResponseWriter, r *http.Request) {
	orgID, ok := adminID(w, r)
	if !ok {
		return
	}
	if _, err := h.repo.GetOrganizationByID(r.Context(), orgID); err != nil {
		adminResult(w, r, "не удалось получить организацию", err)
		return
	}
	h.listInvites(w, r, orgID, true)
}

func (h OrganizationHandler) listInvites(w http.ResponseWriter, r *http.Request, orgID int64, canIssueManagerInvite bool) {
	invites, err := h.repo.ListOrganizationInvites(r.Context(), orgID)
	if err != nil {
		serverError(w, r, "не удалось получить приглашения", err)
		return
	}
	if invites == nil {
		invites = []domain.OrganizationInvite{}
	}
	if !canIssueManagerInvite {
		for i := range invites {
			if invites[i].Role == domain.RoleManager {
				invites[i].Code = ""
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	pdfWriteJSON(w, http.StatusOK, invites)
}

func (h OrganizationHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	if orgID, ok := requireOrgID(w, r); ok {
		h.createInvite(w, r, orgID)
	}
}

func (h OrganizationHandler) CreateAdminInvite(w http.ResponseWriter, r *http.Request) {
	if orgID, ok := adminID(w, r); ok {
		h.createInvite(w, r, orgID)
	}
}

func (h OrganizationHandler) createInvite(w http.ResponseWriter, r *http.Request, orgID int64) {
	actor, _ := currentUser(r)
	var req inviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}
	switch req.Role {
	case domain.RoleMaster, domain.RoleWorker:
	case domain.RoleManager:
		if !actor.IsSuperAdmin {
			http.Error(w, "приглашение руководителя доступно только супер-админу", http.StatusForbidden)
			return
		}
	default:
		http.Error(w, "неизвестная роль", http.StatusBadRequest)
		return
	}
	maxUses, days := 1, 7
	if req.MaxUses != nil {
		maxUses = *req.MaxUses
	}
	if req.ExpiresInDays != nil {
		days = *req.ExpiresInDays
	}
	if maxUses < 1 || maxUses > 1<<31-1 || days < 1 || days > 365 {
		http.Error(w, "укажите положительный лимит использований и срок от 1 до 365 дней", http.StatusBadRequest)
		return
	}
	invite, err := h.issueInvite(r.Context(), orgID, actor.UserID, req.Role, maxUses, days)
	if err != nil {
		adminResult(w, r, "не удалось создать приглашение", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	pdfWriteJSON(w, http.StatusCreated, invite)
}

func (h OrganizationHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgID(w, r)
	if !ok {
		return
	}
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	revoked, err := h.repo.RevokeInvite(r.Context(), orgID, id)
	writeInviteRevocation(w, r, revoked, err)
}

func (h OrganizationHandler) RevokeAdminInvite(w http.ResponseWriter, r *http.Request) {
	orgID, ok := adminID(w, r)
	if !ok {
		return
	}
	inviteID, ok := recordID(w, r, "inviteID")
	if !ok {
		return
	}
	revoked, err := h.repo.RevokeAdminInvite(r.Context(), orgID, inviteID)
	writeInviteRevocation(w, r, revoked, err)
}

func writeInviteRevocation(w http.ResponseWriter, r *http.Request, revoked bool, err error) {
	if err == nil && !revoked {
		http.Error(w, "приглашение не найдено или уже отозвано", http.StatusNotFound)
		return
	}
	adminResult(w, r, "не удалось отозвать приглашение", err)
}

type organizationRequest struct {
	Name                string `json:"name"`
	CompanyInfo         string `json:"company_info"`
	IsDemo              bool   `json:"is_demo"`
	IsActive            *bool  `json:"is_active"`
	CreateManagerInvite *bool  `json:"create_manager_invite"`
}

func readOrganizationRequest(w http.ResponseWriter, r *http.Request) (organizationRequest, bool) {
	var req organizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	req.CompanyInfo = strings.TrimSpace(req.CompanyInfo)
	if req.Name == "" {
		http.Error(w, "укажите название организации", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func (h OrganizationHandler) List(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.repo.ListOrganizations(r.Context())
	if err != nil {
		serverError(w, r, "не удалось получить организации", err)
		return
	}
	pdfWriteJSON(w, http.StatusOK, orgs)
}

func (h OrganizationHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := readOrganizationRequest(w, r)
	if !ok {
		return
	}
	org, err := h.repo.CreateOrganization(r.Context(), req.Name, req.CompanyInfo, req.IsDemo)
	if err != nil {
		serverError(w, r, "не удалось создать организацию", err)
		return
	}
	out := map[string]any{"organization": org, "manager_invite": nil}
	if req.CreateManagerInvite == nil || *req.CreateManagerInvite {
		actor, _ := currentUser(r)
		invite, err := h.issueInvite(r.Context(), org.ID, actor.UserID, domain.RoleManager, 1, 7)
		if err != nil {
			log.Printf("Не удалось создать приглашение руководителя организации %d: %v", org.ID, err)
			out["manager_invite_error"] = "организация создана, создайте приглашение руководителя отдельно"
		} else {
			out["manager_invite"] = invite
			w.Header().Set("Cache-Control", "no-store")
		}
	}
	pdfWriteJSON(w, http.StatusCreated, out)
}

func (h OrganizationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := adminID(w, r)
	if !ok {
		return
	}
	req, ok := readOrganizationRequest(w, r)
	if !ok {
		return
	}
	if req.IsActive == nil {
		http.Error(w, "укажите, активна ли организация", http.StatusBadRequest)
		return
	}
	org, err := h.repo.UpdateOrganization(r.Context(), id, req.Name, req.CompanyInfo, req.IsDemo, *req.IsActive)
	if err != nil {
		adminResult(w, r, "не удалось обновить организацию", err)
		return
	}
	pdfWriteJSON(w, http.StatusOK, org)
}

func (h OrganizationHandler) Reset(w http.ResponseWriter, r *http.Request) {
	if orgID, ok := adminID(w, r); ok {
		h.resetOrganization(w, r, orgID)
	}
}

var resetConfirmationLetters = strings.NewReplacer("C", "С", "B", "Б", "P", "Р", "O", "О")

func (h OrganizationHandler) resetOrganization(w http.ResponseWriter, r *http.Request, orgID int64) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
		return
	}
	org, err := h.repo.GetOrganizationByID(r.Context(), orgID)
	if err != nil {
		adminResult(w, r, "не удалось получить организацию", err)
		return
	}
	confirm := strings.TrimSpace(req.Confirm)
	if confirm == "" || (resetConfirmationLetters.Replace(strings.ToUpper(confirm)) != "СБРОС" && confirm != org.Name) {
		http.Error(w, "для подтверждения введите СБРОС или название организации", http.StatusBadRequest)
		return
	}
	adminResult(w, r, "не удалось очистить тестовые данные", h.repo.ResetTestData(r.Context(), orgID))
}
