package rbac_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/pkg/models"
)

func TestRBACPermissionsAndCASLCan(t *testing.T) {
	pe := rbac.NewPolicyEngine()

	// Owner: full manage on everything
	if !pe.Can(models.RoleOwner, rbac.ActionDelete, rbac.ResourceWorkspace) {
		t.Error("expected owner to be able to delete workspace")
	}
	if !pe.Can(models.RoleOwner, rbac.ActionManage, rbac.ResourceBilling) {
		t.Error("expected owner to be able to manage billing")
	}

	// Admin: can manage rules, but cannot delete workspace
	if !pe.Can(models.RoleAdmin, rbac.ActionCreate, rbac.ResourceRules) {
		t.Error("expected admin to be able to create rules")
	}
	if !pe.Can(models.RoleAdmin, rbac.ActionUpdate, rbac.ResourceRules) {
		t.Error("expected admin to be able to update rules")
	}
	if pe.Can(models.RoleAdmin, rbac.ActionDelete, rbac.ResourceWorkspace) {
		t.Error("expected admin to be DENIED deleting workspace")
	}

	// Member: can create reviews, but cannot create rules
	if !pe.Can(models.RoleMember, rbac.ActionCreate, rbac.ResourceReviews) {
		t.Error("expected member to be able to create reviews")
	}
	if pe.Can(models.RoleMember, rbac.ActionCreate, rbac.ResourceRules) {
		t.Error("expected member to be DENIED creating rules")
	}

	// Viewer: read only
	if !pe.Can(models.RoleViewer, rbac.ActionRead, rbac.ResourceReviews) {
		t.Error("expected viewer to be able to read reviews")
	}
	if pe.Can(models.RoleViewer, rbac.ActionCreate, rbac.ResourceReviews) {
		t.Error("expected viewer to be DENIED creating reviews")
	}
}

func TestRBACMiddlewareEnforcement(t *testing.T) {
	pe := rbac.NewPolicyEngine()
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})

	mw := rbac.RequirePolicy(pe, rbac.ActionCreate, rbac.ResourceRules)(nextHandler)

	// 1. Unauthorized when missing context
	reqUnauth := httptest.NewRequest("POST", "/rules", nil)
	rrUnauth := httptest.NewRecorder()
	mw.ServeHTTP(rrUnauth, reqUnauth)
	if rrUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rrUnauth.Code)
	}

	// 2. Forbidden when Member tries to create rule
	memberProfile := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Role:        models.RoleMember,
	}
	ctxMember := auth.WithAccountContext(context.Background(), memberProfile)
	reqMember := httptest.NewRequest("POST", "/rules", nil).WithContext(ctxMember)
	rrMember := httptest.NewRecorder()
	mw.ServeHTTP(rrMember, reqMember)
	if rrMember.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for member creating rule, got %d", rrMember.Code)
	}

	// 3. Allowed when Admin creates rule
	adminProfile := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Role:        models.RoleAdmin,
	}
	ctxAdmin := auth.WithAccountContext(context.Background(), adminProfile)
	reqAdmin := httptest.NewRequest("POST", "/rules", nil).WithContext(ctxAdmin)
	rrAdmin := httptest.NewRecorder()
	mw.ServeHTTP(rrAdmin, reqAdmin)
	if rrAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin creating rule, got %d", rrAdmin.Code)
	}
}
