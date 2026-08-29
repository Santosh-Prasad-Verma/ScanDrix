package identity

import (
	"time"

	"github.com/google/uuid"
)

// UserRole defines hierarchical organizational privileges.
type UserRole string

const (
	RoleAdmin      UserRole = "admin"
	RoleMaintainer UserRole = "maintainer"
	RoleReviewer   UserRole = "reviewer"
	RoleViewer     UserRole = "viewer"
)

// ResourceAction defines actionable operations on platform entities.
type ResourceAction string

const (
	ActionRead    ResourceAction = "read"
	ActionCreate  ResourceAction = "create"
	ActionUpdate  ResourceAction = "update"
	ActionDelete  ResourceAction = "delete"
	ActionTrigger ResourceAction = "trigger"
)

// ResourceType categorizes platform assets protected by RBAC.
type ResourceType string

const (
	ResourceWorkspace ResourceType = "workspace"
	ResourceRepo      ResourceType = "repository"
	ResourceReview    ResourceType = "code_review"
	ResourceBYOK      ResourceType = "byok_credentials"
	ResourceRule      ResourceType = "security_rule"
)

// UserProfile represents an authenticated user's account details.
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
