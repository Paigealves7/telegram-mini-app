package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"telegram-mini-app/internal/domain"
)

func TestSelectOrganization(t *testing.T) {
	first := organizationOption{ID: 7, IsActive: true, Role: domain.RoleWorker}
	second := organizationOption{ID: 8, IsActive: true, Role: domain.RoleMaster}
	inactive := organizationOption{ID: 9, IsActive: false, Role: domain.RoleManager}
	for _, tc := range []struct {
		name       string
		options    []organizationOption
		super      bool
		header     string
		wantID     int64
		wantStatus int
	}{
		{name: "автовыбор членства", options: []organizationOption{first}, wantID: first.ID},
		{name: "ожидание приглашения", wantStatus: http.StatusForbidden},
		{name: "явный выбор членства", options: []organizationOption{first, second}, header: "8", wantID: second.ID},
		{name: "чужая организация", options: []organizationOption{first}, header: "8", wantStatus: http.StatusForbidden},
		{name: "некорректный заголовок", options: []organizationOption{first}, header: "0", wantStatus: http.StatusBadRequest},
		{name: "явный выбор супер-админа", options: []organizationOption{first, inactive}, super: true, wantStatus: http.StatusConflict},
		{name: "единственная организация супер-админа", options: []organizationOption{first}, super: true, wantID: first.ID},
		{name: "несуществующая организация", options: []organizationOption{first}, super: true, header: "8", wantStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/boards", nil)
			if tc.header != "" {
				req.Header.Set("X-Organization-ID", tc.header)
			}
			org, status, message := selectOrganization(req, tc.options, tc.super)
			if status != tc.wantStatus {
				t.Fatalf("статус %d, ожидали %d (%s)", status, tc.wantStatus, message)
			}
			if tc.wantID == 0 && org != nil || tc.wantID != 0 && (org == nil || org.ID != tc.wantID) {
				t.Fatalf("организация %+v, ожидали ID %d", org, tc.wantID)
			}
		})
	}
}
