package rbac

import (
	"errors"

	"github.com/scandrix/backend/pkg/models"
)

// Action represents operations in the CASL Action enum.
type Action string

const (
	ActionManage Action = "manage" // wildcard full authority
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// Resource represents target securable domain entities.
type Resource string

const (
	ResourceAll          Resource = "all"
	ResourceWorkspace    Resource = "workspace"
	ResourceRepository   Resource = "repository"
	ResourceRules        Resource = "rules"
	ResourceReviews      Resource = "reviews"
	ResourceBilling      Resource = "billing"
	ResourceAuditLogs    Resource = "audit_logs"
	ResourceIntegrations Resource = "integrations"
	ResourceMembers      Resource = "members"
)

// Permission represents legacy discrete securable action in Scandrix.
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

// PolicyRule defines an allowed action on a domain resource.
type PolicyRule struct {
	Action   Action
	Resource Resource
}

var roleMatrix = map[models.UserRole][]PolicyRule{
	models.RoleOwner: {
		{Action: ActionManage, Resource: ResourceAll},
	},
	models.RoleAdmin: {
		{Action: ActionManage, Resource: ResourceRules},
		{Action: ActionManage, Resource: ResourceReviews},
		{Action: ActionManage, Resource: ResourceRepository},
		{Action: ActionManage, Resource: ResourceIntegrations},
		{Action: ActionManage, Resource: ResourceMembers},
		{Action: ActionRead, Resource: ResourceAuditLogs},
		{Action: ActionRead, Resource: ResourceWorkspace},
		{Action: ActionUpdate, Resource: ResourceWorkspace},
		{Action: ActionRead, Resource: ResourceBilling},
	},
	models.RoleMember: {
		{Action: ActionCreate, Resource: ResourceReviews},
		{Action: ActionRead, Resource: ResourceReviews},
		{Action: ActionUpdate, Resource: ResourceReviews},
		{Action: ActionRead, Resource: ResourceRules},
		{Action: ActionRead, Resource: ResourceRepository},
		{Action: ActionRead, Resource: ResourceWorkspace},
	},
	models.RoleViewer: {
		{Action: ActionRead, Resource: ResourceReviews},
		{Action: ActionRead, Resource: ResourceRules},
		{Action: ActionRead, Resource: ResourceRepository},
		{Action: ActionRead, Resource: ResourceWorkspace},
	},
}

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

// PolicyEngine enforces role permissions and CASL attribute checks.
type PolicyEngine struct{}

// NewPolicyEngine initializes the RBAC authorization engine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// Can checks if a given user role has permission to perform an action on a resource.
func (p *PolicyEngine) Can(role models.UserRole, action Action, resource Resource) bool {
	rules, exists := roleMatrix[role]
	if !exists {
		return false
	}

	for _, r := range rules {
		// 1. Full manage wildcard on ResourceAll (e.g. Owner)
		if r.Action == ActionManage && r.Resource == ResourceAll {
			return true
		}
		// 2. Full manage wildcard on specific resource
		if r.Action == ActionManage && r.Resource == resource {
			return true
		}
		// 3. Exact match on action & resource
		if (r.Action == action || r.Action == ActionManage) && r.Resource == resource {
			return true
		}
	}
	return false
}

// CheckPermission validates if a given user role possesses the required legacy permission.
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
	return p.Can(role, ActionUpdate, ResourceRules)
}

// CanAccessAuditLogs verifies if the caller can review audit trails.
func (p *PolicyEngine) CanAccessAuditLogs(role models.UserRole) bool {
	return p.Can(role, ActionRead, ResourceAuditLogs)
}

// GetRolePermissions returns all active permissions for a given role.
func (p *PolicyEngine) GetRolePermissions(role models.UserRole) []string {
	perms, found := rolePermissions[role]
	if !found {
		return []string{}
	}
	res := make([]string, 0, len(perms))
	for k, v := range perms {
		if v {
			res = append(res, string(k))
		}
	}
	return res
}
