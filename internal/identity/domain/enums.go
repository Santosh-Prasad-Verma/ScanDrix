package domain

// Action represents an operation that can be executed on a resource.
type Action string

const (
	ActionManage Action = "manage" // Wildcard for any action
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// Role represents organization-level user privileges.
type Role string

const (
	RoleOwner          Role = "owner"
	RoleBillingManager Role = "billing_manager"
	RoleRepoAdmin      Role = "repo_admin"
	RoleContributor    Role = "contributor"
)

// ResourceType categorizes platform assets subject to authorization.
type ResourceType string

const (
	ResourceAll                ResourceType = "all"
	ResourcePullRequests       ResourceType = "pull_requests"
	ResourceIssues             ResourceType = "issues"
	ResourceCockpit            ResourceType = "cockpit"
	ResourceBilling            ResourceType = "billing"
	ResourceCodeReviewSettings ResourceType = "code_review_settings"
	ResourceIssuesSettings     ResourceType = "issues_settings"
	ResourceGitSettings        ResourceType = "git_settings"
	ResourceUserSettings       ResourceType = "user_settings"
	ResourceOrgSettings        ResourceType = "organization_settings"
	ResourcePluginSettings     ResourceType = "plugin_settings"
	ResourceLogs               ResourceType = "logs"
	ResourceDrixyRules         ResourceType = "drixy_rules"
	ResourceTokenUsage         ResourceType = "token_usage"
	ResourceCliReview          ResourceType = "cli_review"
)

// PolicyScope defines whether an authorization rule applies organization-wide or per-repo.
type PolicyScope string

const (
	ScopeOrg  PolicyScope = "org"
	ScopeRepo PolicyScope = "repo"
)

// PolicyRule defines a single declarative authorization grant.
type PolicyRule struct {
	Action   Action       `json:"action"`
	Resource ResourceType `json:"resource"`
	Scope    PolicyScope  `json:"scope"`
	Global   bool         `json:"global,omitempty"`
}

// OwnerPolicy grants unrestricted management across all organization resources.
var OwnerPolicy = []PolicyRule{
	{Action: ActionManage, Resource: ResourceAll, Scope: ScopeOrg},
}

// RepoAdminPolicy grants broad read access across the organization and write/mutate
// access to assigned repositories and issue settings.
var RepoAdminPolicy = []PolicyRule{
	// Code review settings: read org-wide, edit assigned repos.
	{Action: ActionRead, Resource: ResourceCodeReviewSettings, Scope: ScopeOrg},
	{Action: ActionUpdate, Resource: ResourceCodeReviewSettings, Scope: ScopeRepo},
	{Action: ActionCreate, Resource: ResourceCodeReviewSettings, Scope: ScopeRepo},

	// Drixy rules: read org-wide, edit assigned repos.
	{Action: ActionRead, Resource: ResourceDrixyRules, Scope: ScopeOrg},
	{Action: ActionUpdate, Resource: ResourceDrixyRules, Scope: ScopeRepo},
	{Action: ActionCreate, Resource: ResourceDrixyRules, Scope: ScopeRepo},
	{Action: ActionDelete, Resource: ResourceDrixyRules, Scope: ScopeRepo},

	// Cockpit: read only (cockpit settings are owner-only).
	{Action: ActionRead, Resource: ResourceCockpit, Scope: ScopeOrg},

	// Issues: read across the org, create/update on assigned repos.
	{Action: ActionRead, Resource: ResourceIssues, Scope: ScopeOrg},
	{Action: ActionUpdate, Resource: ResourceIssues, Scope: ScopeRepo},
	{Action: ActionCreate, Resource: ResourceIssues, Scope: ScopeRepo},

	// Issues settings: org-wide configuration.
	{Action: ActionRead, Resource: ResourceIssuesSettings, Scope: ScopeOrg},
	{Action: ActionUpdate, Resource: ResourceIssuesSettings, Scope: ScopeOrg},
	{Action: ActionCreate, Resource: ResourceIssuesSettings, Scope: ScopeOrg},

	{Action: ActionRead, Resource: ResourceLogs, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourcePullRequests, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceGitSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourcePluginSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceTokenUsage, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceCliReview, Scope: ScopeOrg},
}

// BillingManagerPolicy grants access to billing management and read access to logs/settings.
var BillingManagerPolicy = []PolicyRule{
	{Action: ActionRead, Resource: ResourceCodeReviewSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceDrixyRules, Scope: ScopeOrg},
	{Action: ActionManage, Resource: ResourceBilling, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceGitSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourcePluginSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceUserSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceIssuesSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceLogs, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceTokenUsage, Scope: ScopeOrg},
}

// ContributorPolicy is a read-only role seeing settings, rules, issues, and PRs org-wide.
var ContributorPolicy = []PolicyRule{
	{Action: ActionRead, Resource: ResourceCodeReviewSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceDrixyRules, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceIssues, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceIssuesSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceCliReview, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourcePullRequests, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceLogs, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourceGitSettings, Scope: ScopeOrg},
	{Action: ActionRead, Resource: ResourcePluginSettings, Scope: ScopeOrg},
}

// RolePolicies maps each Role to its declarative authorization rules.
var RolePolicies = map[Role][]PolicyRule{
	RoleOwner:          OwnerPolicy,
	RoleRepoAdmin:      RepoAdminPolicy,
	RoleBillingManager: BillingManagerPolicy,
	RoleContributor:    ContributorPolicy,
}

// RoleUsesRepoAssignment returns true if any rule for the role is repo-scoped.
func RoleUsesRepoAssignment(role Role) bool {
	rules, exists := RolePolicies[role]
	if !exists {
		return false
	}
	for _, r := range rules {
		if r.Scope == ScopeRepo {
			return true
		}
	}
	return false
}

// ProfileConfigKey represents individual profile configuration items.
type ProfileConfigKey string

const (
	ProfileConfigUserNotifications ProfileConfigKey = "user_notifications"
)

// CliAuthSessionMode specifies how the CLI authenticates.
type CliAuthSessionMode string

const (
	CliAuthModeLoopback CliAuthSessionMode = "loopback"
	CliAuthModeDevice   CliAuthSessionMode = "device"
)

// CliAuthSessionStatus represents the lifecycle state of a CLI login session.
type CliAuthSessionStatus string

const (
	CliAuthStatusPending   CliAuthSessionStatus = "pending"
	CliAuthStatusCompleted CliAuthSessionStatus = "completed"
	CliAuthStatusConsumed  CliAuthSessionStatus = "consumed"
	CliAuthStatusDenied    CliAuthSessionStatus = "denied"
	CliAuthStatusExpired   CliAuthSessionStatus = "expired"
)

// AuthProvider identifies third-party or local identity providers.
type AuthProvider string

const (
	AuthProviderCredentials AuthProvider = "credentials"
	AuthProviderGoogle      AuthProvider = "google"
	AuthProviderGitHub      AuthProvider = "github"
	AuthProviderGitLab      AuthProvider = "gitlab"
)

// UserStatus represents user account lifecycle status.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusPending  UserStatus = "pending"
	UserStatusInactive UserStatus = "inactive"
)

// TeamMemberRole specifies membership level within a team.
type TeamMemberRole string

const (
	TeamMemberRoleLeader TeamMemberRole = "team_leader"
	TeamMemberRoleMember TeamMemberRole = "member"
)
