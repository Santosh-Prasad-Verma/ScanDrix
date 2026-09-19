package identity

import (
	"context"
	"sync"

	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
)

// PermissionsEngine enforces RBAC and CASL ability matrices across platform resources.
type PermissionsEngine struct {
	mu             sync.RWMutex
	policies       map[UserRole]PermissionPolicy
	abilityFactory *application.PermissionsAbilityFactory
}

// NewPermissionsEngine constructs an engine initialized with default policies.
func NewPermissionsEngine() *PermissionsEngine {
	e := &PermissionsEngine{
		policies:       make(map[UserRole]PermissionPolicy),
		abilityFactory: application.NewPermissionsAbilityFactory(nil),
	}
	e.loadDefaultPolicies()
	return e
}

func (e *PermissionsEngine) loadDefaultPolicies() {
	// Admin / Owner: Unrestricted permissions
	e.policies[RoleAdmin] = PermissionPolicy{
		Role: RoleAdmin,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace:          {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceRepo:               {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceReview:             {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceBYOK:               {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceRule:               {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceAll:                {ActionManage, ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceOrgSettings:        {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceCodeReviewSettings: {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourceCliReview:          {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionTrigger},
			ResourcePluginSettings:     {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceDrixyRules:         {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
		},
	}
	e.policies[RoleOwner] = e.policies[RoleAdmin]

	// Maintainer: Can manage repos and reviews, cannot delete workspace or edit BYOK
	e.policies[RoleMaintainer] = PermissionPolicy{
		Role: RoleMaintainer,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace:          {ActionRead},
			ResourceRepo:               {ActionRead, ActionCreate, ActionUpdate, ActionTrigger},
			ResourceReview:             {ActionRead, ActionCreate, ActionUpdate, ActionTrigger},
			ResourceBYOK:               {ActionRead},
			ResourceRule:               {ActionRead, ActionCreate, ActionUpdate},
			ResourceOrgSettings:        {ActionRead},
			ResourceCodeReviewSettings: {ActionRead, ActionCreate, ActionUpdate, ActionTrigger},
			ResourceCliReview:          {ActionRead, ActionCreate, ActionUpdate, ActionTrigger},
			ResourcePluginSettings:     {ActionRead},
			ResourceDrixyRules:         {ActionRead, ActionCreate, ActionUpdate, ActionDelete},
			ResourceIssues:             {ActionRead, ActionCreate, ActionUpdate},
			ResourceIssuesSettings:     {ActionRead, ActionCreate, ActionUpdate},
		},
	}
	e.policies[RoleRepoAdmin] = e.policies[RoleMaintainer]

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

	// Viewer: Read-only access across platform resources
	e.policies[RoleViewer] = PermissionPolicy{
		Role: RoleViewer,
		Allowed: map[ResourceType][]ResourceAction{
			ResourceWorkspace:          {ActionRead},
			ResourceRepo:               {ActionRead},
			ResourceReview:             {ActionRead},
			ResourceRule:               {ActionRead},
			ResourceOrgSettings:        {ActionRead},
			ResourceCodeReviewSettings: {ActionRead},
			ResourceCliReview:          {ActionRead},
			ResourceDrixyRules:         {ActionRead},
			ResourceIssues:             {ActionRead},
			ResourceIssuesSettings:     {ActionRead},
			ResourcePullRequests:       {ActionRead},
			ResourceLogs:               {ActionRead},
		},
	}
	e.policies[RoleContributor] = e.policies[RoleViewer]
}

// Can checks whether a user with their role is permitted to perform the specified action.
func (e *PermissionsEngine) Can(user UserProfile, action ResourceAction, resource ResourceType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. First check local policies map
	if policy, exists := e.policies[user.Role]; exists {
		if allowedActions, hasResource := policy.Allowed[resource]; hasResource {
			for _, a := range allowedActions {
				if a == ActionManage || a == action {
					return true
				}
			}
		}
	}

	// 2. Check against CASL ability factory if role matches a domain role
	domainRole := domain.Role(user.Role)
	if _, valid := domain.RolePolicies[domainRole]; valid {
		domainUser := domain.User{
			UUID:             user.ID,
			Role:             domainRole,
			OrganizationUUID: &user.WorkspaceID,
		}

		ability, err := e.abilityFactory.CreateForUser(context.Background(), domainUser, nil)
		if err == nil && ability != nil {
			return ability.Can(domain.Action(action), domain.ResourceType(resource))
		}
	}

	return false
}
