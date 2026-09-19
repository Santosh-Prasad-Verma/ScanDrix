package identity

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// UserRole defines organizational privileges.
type UserRole string

const (
	RoleOwner          UserRole = UserRole(domain.RoleOwner)
	RoleBillingManager UserRole = UserRole(domain.RoleBillingManager)
	RoleRepoAdmin      UserRole = UserRole(domain.RoleRepoAdmin)
	RoleContributor    UserRole = UserRole(domain.RoleContributor)

	// Legacy ScanDrix roles
	RoleAdmin      UserRole = "admin"
	RoleMaintainer UserRole = "maintainer"
	RoleReviewer   UserRole = "reviewer"
	RoleViewer     UserRole = "viewer"
)

// ResourceAction defines operations on resources.
type ResourceAction string

const (
	ActionManage  ResourceAction = ResourceAction(domain.ActionManage)
	ActionCreate  ResourceAction = ResourceAction(domain.ActionCreate)
	ActionRead    ResourceAction = ResourceAction(domain.ActionRead)
	ActionUpdate  ResourceAction = ResourceAction(domain.ActionUpdate)
	ActionDelete  ResourceAction = ResourceAction(domain.ActionDelete)
	ActionTrigger ResourceAction = "trigger"
)

// ResourceType categorizes platform assets protected by authorization policies.
type ResourceType string

const (
	ResourceAll                ResourceType = ResourceType(domain.ResourceAll)
	ResourcePullRequests       ResourceType = ResourceType(domain.ResourcePullRequests)
	ResourceIssues             ResourceType = ResourceType(domain.ResourceIssues)
	ResourceCockpit            ResourceType = ResourceType(domain.ResourceCockpit)
	ResourceBilling            ResourceType = ResourceType(domain.ResourceBilling)
	ResourceCodeReviewSettings ResourceType = ResourceType(domain.ResourceCodeReviewSettings)
	ResourceIssuesSettings     ResourceType = ResourceType(domain.ResourceIssuesSettings)
	ResourceGitSettings        ResourceType = ResourceType(domain.ResourceGitSettings)
	ResourceUserSettings       ResourceType = ResourceType(domain.ResourceUserSettings)
	ResourceOrgSettings        ResourceType = ResourceType(domain.ResourceOrgSettings)
	ResourcePluginSettings     ResourceType = ResourceType(domain.ResourcePluginSettings)
	ResourceLogs               ResourceType = ResourceType(domain.ResourceLogs)
	ResourceDrixyRules         ResourceType = ResourceType(domain.ResourceDrixyRules)
	ResourceTokenUsage         ResourceType = ResourceType(domain.ResourceTokenUsage)
	ResourceCliReview          ResourceType = ResourceType(domain.ResourceCliReview)

	// Legacy aliases
	ResourceWorkspace ResourceType = "workspace"
	ResourceRepo      ResourceType = "repository"
	ResourceReview    ResourceType = "code_review"
	ResourceBYOK      ResourceType = "byok_credentials"
	ResourceRule      ResourceType = "security_rule"
)

// UserProfile represents account details and environment preferences.
type UserProfile struct {
	ID          uuid.UUID         `json:"id"`
	WorkspaceID uuid.UUID         `json:"workspace_id"`
	Email       string            `json:"email"`
	DisplayName string            `json:"display_name"`
	Role        UserRole          `json:"role"`
	Preferences map[string]string `json:"preferences"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// PermissionPolicy maps a role to allowed actions per resource type.
type PermissionPolicy struct {
	Role    UserRole
	Allowed map[ResourceType][]ResourceAction
}
