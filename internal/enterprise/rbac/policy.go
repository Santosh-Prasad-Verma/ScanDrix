package rbac

import (
	"errors"

	"github.com/scandrix/backend/pkg/models"
)

// Permission represents a discrete securable action in Scandrix.
type Permission string

const (
	PermWorkspaceRead    Permission = "workspace:read"
	PermWorkspaceWrite   Permission = "workspace:write"
	PermWorkspaceDelete  Permission = "workspace:delete"
	PermReviewCreate     Permission = "review:create"
	PermReviewRead       Permission = "review:read"
	PermReviewApprove    Permission = "review:approve"
	PermReviewDismiss    Permission = "review:dismiss"
	PermRuleCreate       Permission = "rule:create"
	PermRuleEdit         Permission = "rule:edit"
	PermRuleDelete       Permission = "rule:delete"
	PermIntegrationAdmin Permission = "integration:manage"
	PermAuditRead        Permission = "audit:read"
	PermBillingManage    Permission = "billing:manage"
)

var rolePermissions = map[models.UserRole]map[Permission]bool{
	models.RoleOwner: {
		PermWorkspaceRead:    true,
		PermWorkspaceWrite:   true,
		PermWorkspaceDelete:  true,
		PermReviewCreate:     true,
		PermReviewRead:       true,
		PermReviewApprove:    true,
		PermReviewDismiss:    true,
		PermRuleCreate:       true,
		PermRuleEdit:         true,
		PermRuleDelete:       true,
		PermIntegrationAdmin: true,
		PermAuditRead:        true,
		PermBillingManage:    true,
	},
	models.RoleAdmin: {
		PermWorkspaceRead:    true,
		PermWorkspaceWrite:   true,
		PermReviewCreate:     true,
		PermReviewRead:       true,
		PermReviewApprove:    true,
		PermReviewDismiss:    true,
		PermRuleCreate:       true,
		PermRuleEdit:         true,
		PermRuleDelete:       true,
		PermIntegrationAdmin: true,
		PermAuditRead:        true,
	},
	models.RoleMember: {
		PermWorkspaceRead: true,
		PermReviewCreate:  true,
		PermReviewRead:    true,
		PermReviewApprove: true,
		PermRuleCreate:    false,
	},
	models.RoleViewer: {
		PermWorkspaceRead: true,
		PermReviewRead:    true,
	},
}

// PolicyEngine enforces role permissions and attribute checks.
type PolicyEngine struct{}

// NewPolicyEngine initializes the RBAC authorization engine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// CheckPermission validates if a given user role possesses the required permission.
func (p *PolicyEngine) CheckPermission(role models.UserRole, perm Permission) error {
	perms, found := rolePermissions[role]
	if !found {
		return errors.New("unknown or invalid user role")
	}

	if perms[perm] {
		return nil
	}

	return errors.New("access denied: insufficient role permissions")
}

// CanManageRules verifies whether the caller can create or mutate custom security rules.
func (p *PolicyEngine) CanManageRules(role models.UserRole) bool {
	return p.CheckPermission(role, PermRuleEdit) == nil
}

// CanAccessAuditLogs verifies if the caller can review audit trails.
func (p *PolicyEngine) CanAccessAuditLogs(role models.UserRole) bool {
	return p.CheckPermission(role, PermAuditRead) == nil
}
