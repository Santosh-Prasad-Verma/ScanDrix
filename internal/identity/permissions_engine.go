package identity

import "sync"

// PermissionsEngine enforces RBAC policy matrices across platform resources.
type PermissionsEngine struct {
	mu       sync.RWMutex
	policies map[UserRole]PermissionPolicy
}

func NewPermissionsEngine() *PermissionsEngine {
	e := &PermissionsEngine{
		policies: make(map[UserRole]PermissionPolicy),
	}
	e.loadDefaultPolicies()
	return e
}

func (e *PermissionsEngine) loadDefaultPolicies() {
	// Admin: Unrestricted permissions
	e.policies[RoleAdmin] = PermissionPolicy{
		Role: RoleAdmin,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace: {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceRepo:      {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceReview:    {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceBYOK:      {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceRule:      {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
		},
	}

	// Maintainer: Can manage repos and reviews, but cannot delete workspace or edit BYOK
	e.policies[RoleMaintainer] = PermissionPolicy{
		Role: RoleMaintainer,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace: {ActionRead},
			ResourceRepo:      {ActionRead, ActionCreate, ActionUpdate, ActionTrigger},
			ResourceReview:    {ActionRead, ActionCreate, ActionUpdate, ActionTrigger},
			ResourceBYOK:      {ActionRead},
			ResourceRule:      {ActionRead, ActionCreate, ActionUpdate},
		},
	}

	// Reviewer: Can trigger and read reviews, cannot mutate configs
	e.policies[RoleReviewer] = PermissionPolicy{
		Role: RoleReviewer,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace: {ActionRead},
			ResourceRepo:      {ActionRead},
			ResourceReview:    {ActionRead, ActionTrigger},
			ResourceRule:      {ActionRead},
		},
	}

	// Viewer: Read-only access
	e.policies[RoleViewer] = PermissionPolicy{
		Role: RoleViewer,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace: {ActionRead},
			ResourceRepo:      {ActionRead},
			ResourceReview:    {ActionRead},
			ResourceRule:      {ActionRead},
		},
	}
}

// Can checks whether a user with their role is permitted to perform the specified action.
func (e *PermissionsEngine) Can(user UserProfile, action ResourceAction, resource ResourceType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	policy, exists := e.policies[user.Role]
	if !exists {
		return false
	}

	allowedActions, hasResource := policy.Allowed[resource]
	if !hasResource {
		return false
	}

	for _, a := range allowedActions {
		if a == action {
			return true
		}
	}

	return false
}
