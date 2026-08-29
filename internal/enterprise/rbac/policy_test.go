package rbac_test

import (
	"testing"

	"github.com/scandrix/backend/internal/enterprise/rbac"
	"github.com/scandrix/backend/pkg/models"
)

func TestRBACPermissions(t *testing.T) {
	pe := rbac.NewPolicyEngine()

	// Owner checks
	if err := pe.CheckPermission(models.RoleOwner, rbac.PermWorkspaceDelete); err != nil {
		t.Errorf("expected owner to have PermWorkspaceDelete, got %v", err)
	}

	// Admin checks
	if err := pe.CheckPermission(models.RoleAdmin, rbac.PermRuleCreate); err != nil {
		t.Errorf("expected admin to have PermRuleCreate, got %v", err)
	}
	if err := pe.CheckPermission(models.RoleAdmin, rbac.PermBillingManage); err == nil {
		t.Error("expected admin to be denied PermBillingManage")
	}

	// Member checks
	if err := pe.CheckPermission(models.RoleMember, rbac.PermReviewCreate); err != nil {
		t.Errorf("expected member to have PermReviewCreate, got %v", err)
	}
	if err := pe.CheckPermission(models.RoleMember, rbac.PermRuleCreate); err == nil {
		t.Error("expected member to be denied PermRuleCreate")
	}

	// Viewer checks
	if err := pe.CheckPermission(models.RoleViewer, rbac.PermReviewRead); err != nil {
		t.Errorf("expected viewer to have PermReviewRead, got %v", err)
	}
	if err := pe.CheckPermission(models.RoleViewer, rbac.PermReviewCreate); err == nil {
		t.Error("expected viewer to be denied PermReviewCreate")
	}
}
