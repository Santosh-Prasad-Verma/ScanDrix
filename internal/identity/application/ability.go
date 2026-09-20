package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// ActiveRule represents an instantiated authorization grant with resolved scopes.
type ActiveRule struct {
	Action               domain.Action       `json:"action"`
	Resource             domain.ResourceType `json:"resource"`
	Scope                domain.PolicyScope  `json:"scope"`
	Global               bool                `json:"global"`
	OrganizationUUID     uuid.UUID           `json:"organization_uuid"`
	AllowedRepositoryIDs []string            `json:"allowed_repository_ids,omitempty"`
}

// AppAbility encapsulates the complete set of active permission grants for a user.
type AppAbility struct {
	UserUUID         uuid.UUID
	OrganizationUUID uuid.UUID
	Role             domain.Role
	Rules            []ActiveRule
}

// Can evaluates whether the user is authorized to perform the action on the resource org-wide.
func (a *AppAbility) Can(action domain.Action, resource domain.ResourceType) bool {
	for _, rule := range a.Rules {
		// Action wildcard check: ActionManage covers all actions
		actionMatches := rule.Action == domain.ActionManage || rule.Action == action

		// Resource wildcard check: ResourceAll covers all resources
		resourceMatches := rule.Resource == domain.ResourceAll || rule.Resource == resource

		if actionMatches && resourceMatches {
			return true
		}
	}
	return false
}

// CanInRepo evaluates whether the user is authorized for action on a specific repository.
func (a *AppAbility) CanInRepo(action domain.Action, resource domain.ResourceType, repoID string) bool {
	for _, rule := range a.Rules {
		actionMatches := rule.Action == domain.ActionManage || rule.Action == action
		resourceMatches := rule.Resource == domain.ResourceAll || rule.Resource == resource

		if !actionMatches || !resourceMatches {
			continue
		}

		// Org-scoped rules apply to all repositories in the organization
		if rule.Scope == domain.ScopeOrg {
			return true
		}

		// Repo-scoped rules require the target repo to be in the allowed list
		if rule.Scope == domain.ScopeRepo {
			if rule.Global && repoID == "global" {
				return true
			}
			for _, allowedID := range rule.AllowedRepositoryIDs {
				if allowedID == repoID {
					return true
				}
			}
		}
	}
	return false
}

// BuildPermissionsMap generates the canonical permission matrix for the frontend dashboard.
func (a *AppAbility) BuildPermissionsMap() map[domain.ResourceType]map[domain.Action]any {
	result := make(map[domain.ResourceType]map[domain.Action]any)

	allResources := []domain.ResourceType{
		domain.ResourcePullRequests,
		domain.ResourceIssues,
		domain.ResourceCockpit,
		domain.ResourceBilling,
		domain.ResourceCodeReviewSettings,
		domain.ResourceIssuesSettings,
		domain.ResourceGitSettings,
		domain.ResourceUserSettings,
		domain.ResourceOrgSettings,
		domain.ResourcePluginSettings,
		domain.ResourceLogs,
		domain.ResourceDrixyRules,
		domain.ResourceTokenUsage,
		domain.ResourceCliReview,
	}

	for _, rule := range a.Rules {
		var targets []domain.ResourceType
		if rule.Resource == domain.ResourceAll {
			targets = allResources
		} else {
			targets = []domain.ResourceType{rule.Resource}
		}

		var actions []domain.Action
		if rule.Action == domain.ActionManage {
			actions = []domain.Action{
				domain.ActionManage,
				domain.ActionCreate,
				domain.ActionRead,
				domain.ActionUpdate,
				domain.ActionDelete,
			}
		} else {
			actions = []domain.Action{rule.Action}
		}

		for _, res := range targets {
			if _, exists := result[res]; !exists {
				result[res] = make(map[domain.Action]any)
			}

			var condition any
			if rule.Scope == domain.ScopeRepo {
				repos := make([]string, len(rule.AllowedRepositoryIDs))
				copy(repos, rule.AllowedRepositoryIDs)
				if rule.Global {
					repos = append(repos, "global")
				}
				condition = map[string]any{
					"organizationId": a.OrganizationUUID.String(),
					"repoId": map[string]any{
						"$in": repos,
					},
				}
			} else {
				condition = map[string]any{
					"organizationId": a.OrganizationUUID.String(),
				}
			}

			for _, act := range actions {
				result[res][act] = condition
			}
		}
	}

	return result
}

// PermissionsAbilityFactory instantiates AppAbility objects from user records and policy rules.
type PermissionsAbilityFactory struct {
	permissionsRepo domain.PermissionsRepository
}

// NewPermissionsAbilityFactory creates a new factory instance.
func NewPermissionsAbilityFactory(repo domain.PermissionsRepository) *PermissionsAbilityFactory {
	return &PermissionsAbilityFactory{
		permissionsRepo: repo,
	}
}

// CreateForUser builds an AppAbility instance for the specified user and assigned repositories.
func (f *PermissionsAbilityFactory) CreateForUser(ctx context.Context, user domain.User, repositoryIDs []string) (*AppAbility, error) {
	ability := &AppAbility{
		UserUUID: user.UUID,
		Role:     user.Role,
	}

	if user.OrganizationUUID != nil {
		ability.OrganizationUUID = *user.OrganizationUUID
	}

	if user.Role == "" || user.OrganizationUUID == nil {
		return ability, nil
	}

	var assignedRepoIDs []string
	if repositoryIDs != nil {
		assignedRepoIDs = repositoryIDs
	} else if f.permissionsRepo != nil {
		perms, err := f.permissionsRepo.FindByUserUUID(ctx, user.UUID)
		if err == nil && perms != nil {
			assignedRepoIDs = perms.AssignedRepositoryIDs
		}
	}

	policyRules, exists := domain.RolePolicies[user.Role]
	if !exists {
		return ability, nil
	}

	activeRules := make([]ActiveRule, 0, len(policyRules))
	for _, pr := range policyRules {
		rule := ActiveRule{
			Action:           pr.Action,
			Resource:         pr.Resource,
			Scope:            pr.Scope,
			Global:           pr.Global,
			OrganizationUUID: ability.OrganizationUUID,
		}
		if pr.Scope == domain.ScopeRepo {
			rule.AllowedRepositoryIDs = assignedRepoIDs
		}
		activeRules = append(activeRules, rule)
	}

	ability.Rules = activeRules
	return ability, nil
}
