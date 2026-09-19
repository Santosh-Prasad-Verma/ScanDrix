package infrastructure

import (
	"context"

	"github.com/scandrix/backend/internal/identity/domain"
)

// PolicyScope defines whether a grant applies organization-wide or per-assigned-repository.
type PolicyScope string

const (
	PolicyScopeOrg  PolicyScope = "org"
	PolicyScopeRepo PolicyScope = "repo"
)

// PolicyRule defines an atomic authorization entitlement.
type PolicyRule struct {
	Action   domain.Action       `json:"action"`
	Resource domain.ResourceType `json:"resource"`
	Scope    PolicyScope         `json:"scope"`
	Global   bool                `json:"global,omitempty"`
}

// Default role policies in ScanDrix.
var (
	OwnerPolicy = []PolicyRule{
		{Action: domain.ActionManage, Resource: domain.ResourceAll, Scope: PolicyScopeOrg},
	}

	RepoAdminPolicy = []PolicyRule{
		{Action: domain.ActionRead, Resource: domain.ResourceCodeReviewSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionUpdate, Resource: domain.ResourceCodeReviewSettings, Scope: PolicyScopeRepo},
		{Action: domain.ActionCreate, Resource: domain.ResourceCodeReviewSettings, Scope: PolicyScopeRepo},

		{Action: domain.ActionRead, Resource: domain.ResourceDrixyRules, Scope: PolicyScopeOrg},
		{Action: domain.ActionUpdate, Resource: domain.ResourceDrixyRules, Scope: PolicyScopeRepo},
		{Action: domain.ActionCreate, Resource: domain.ResourceDrixyRules, Scope: PolicyScopeRepo},
		{Action: domain.ActionDelete, Resource: domain.ResourceDrixyRules, Scope: PolicyScopeRepo},

		{Action: domain.ActionRead, Resource: domain.ResourceCockpit, Scope: PolicyScopeOrg},

		{Action: domain.ActionRead, Resource: domain.ResourceIssues, Scope: PolicyScopeOrg},
		{Action: domain.ActionUpdate, Resource: domain.ResourceIssues, Scope: PolicyScopeRepo},
		{Action: domain.ActionCreate, Resource: domain.ResourceIssues, Scope: PolicyScopeRepo},

		{Action: domain.ActionRead, Resource: domain.ResourceIssuesSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionUpdate, Resource: domain.ResourceIssuesSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionCreate, Resource: domain.ResourceIssuesSettings, Scope: PolicyScopeOrg},

		{Action: domain.ActionRead, Resource: domain.ResourceLogs, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourcePullRequests, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceGitSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourcePluginSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceTokenUsage, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceCliReview, Scope: PolicyScopeOrg},
	}

	BillingManagerPolicy = []PolicyRule{
		{Action: domain.ActionRead, Resource: domain.ResourceCodeReviewSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceDrixyRules, Scope: PolicyScopeOrg},
		{Action: domain.ActionManage, Resource: domain.ResourceBilling, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceGitSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourcePluginSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceUserSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceIssuesSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceLogs, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceTokenUsage, Scope: PolicyScopeOrg},
	}

	ContributorPolicy = []PolicyRule{
		{Action: domain.ActionRead, Resource: domain.ResourceCodeReviewSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceDrixyRules, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceIssues, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceIssuesSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceCliReview, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourcePullRequests, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceLogs, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourceGitSettings, Scope: PolicyScopeOrg},
		{Action: domain.ActionRead, Resource: domain.ResourcePluginSettings, Scope: PolicyScopeOrg},
	}

	RolePolicies = map[domain.Role][]PolicyRule{
		domain.RoleOwner:          OwnerPolicy,
		domain.RoleRepoAdmin:      RepoAdminPolicy,
		domain.RoleBillingManager: BillingManagerPolicy,
		domain.RoleContributor:    ContributorPolicy,
	}
)

// RoleUsesRepoAssignment returns true if at least one permission rule is scoped to repository.
func RoleUsesRepoAssignment(role domain.Role) bool {
	rules, ok := RolePolicies[role]
	if !ok {
		return false
	}
	for _, r := range rules {
		if r.Scope == PolicyScopeRepo {
			return true
		}
	}
	return false
}

// PolicyHandler evaluates whether a user context satisfies a specific policy check.
type PolicyHandler interface {
	Handle(ctx context.Context, ability *domain.AppAbility, targetRepoID string) bool
}

// GenericPolicyHandler checks action and subject against ability with optional repository scope.
type GenericPolicyHandler struct {
	Action   domain.Action
	Subject  domain.ResourceType
	Requires string
}

func (h *GenericPolicyHandler) Handle(ctx context.Context, ability *domain.AppAbility, targetRepoID string) bool {
	if ability == nil {
		return false
	}
	if ability.IsAdmin {
		return true
	}

	actionStr := string(h.Action)
	subjStr := string(h.Subject)

	// Check org-wide entitlement
	if ability.Can(actionStr, subjStr) {
		return true
	}

	// Check repository-scoped entitlement
	if targetRepoID != "" && ability.Can(actionStr, subjStr) {
		if ability.CanAccessRepo(targetRepoID) {
			return true
		}
	}

	return false
}

// PolicyGuard intercepts requests and enforces policy handler rules.
type PolicyGuard struct {
	abilityFactory *AbilityFactory
}

// NewPolicyGuard creates a new PolicyGuard.
func NewPolicyGuard(factory *AbilityFactory) *PolicyGuard {
	return &PolicyGuard{abilityFactory: factory}
}

// CanActivate evaluates all required policy handlers for a user.
func (g *PolicyGuard) CanActivate(
	ctx context.Context,
	user domain.User,
	role domain.Role,
	assignedRepos []string,
	handlers []PolicyHandler,
	targetRepoID string,
) bool {
	ability := g.abilityFactory.CreateForUser(user, role, assignedRepos)
	for _, h := range handlers {
		if !h.Handle(ctx, ability, targetRepoID) {
			return false
		}
	}
	return true
}

// AbilityFactory synthesizes an AppAbility from role definitions and database repository assignments.
type AbilityFactory struct{}

// NewAbilityFactory creates an AbilityFactory.
func NewAbilityFactory() *AbilityFactory {
	return &AbilityFactory{}
}

// CreateForUser compiles PolicyRules and assigned repos into a ready-to-query AppAbility.
func (f *AbilityFactory) CreateForUser(user domain.User, role domain.Role, assignedRepos []string) *domain.AppAbility {
	ability := domain.NewAppAbility(user.IsAdmin)

	rules, exists := RolePolicies[role]
	if !exists {
		rules = ContributorPolicy
	}

	for _, rule := range rules {
		act := string(rule.Action)
		res := string(rule.Resource)

		if rule.Action == domain.ActionManage && rule.Resource == domain.ResourceAll {
			ability.Allow("*", "*")
			continue
		}

		if rule.Scope == PolicyScopeOrg {
			ability.Allow(act, res)
		} else if rule.Scope == PolicyScopeRepo {
			// Attach conditions checking repository ID
			for _, repoID := range assignedRepos {
				ability.AllowWithRepo(act, res, repoID)
			}
		}
	}

	for _, r := range assignedRepos {
		ability.AssignRepo(r)
	}

	return ability
}
