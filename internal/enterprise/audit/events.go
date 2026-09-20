// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package audit

import (
	"time"

	"github.com/google/uuid"
)

// AuditEventCategory classifies enterprise actions into compliance domains.
type AuditEventCategory string

const (
	CategoryCodeReviewConfig         AuditEventCategory = "CODE_REVIEW_CONFIG"
	CategoryDrixyRules               AuditEventCategory = "DRIXY_RULES"
	CategoryRepositories             AuditEventCategory = "REPOSITORIES"
	CategoryRepositoryConfigRemoval  AuditEventCategory = "REPOSITORY_CONFIG_REMOVAL"
	CategoryDirectoryConfigRemoval   AuditEventCategory = "DIRECTORY_CONFIG_REMOVAL"
	CategoryIntegration              AuditEventCategory = "INTEGRATION"
	CategoryUserStatus               AuditEventCategory = "USER_STATUS"
	CategoryPRMessages               AuditEventCategory = "PR_MESSAGES"
	CategoryUserInvite               AuditEventCategory = "USER_INVITE"
	CategoryUserRoleChange           AuditEventCategory = "USER_ROLE_CHANGE"
	CategoryUserRepoAccess           AuditEventCategory = "USER_REPO_ACCESS"
	CategoryOrgSettings              AuditEventCategory = "ORG_SETTINGS"
	CategoryCliKey                   AuditEventCategory = "CLI_KEY"
)

// ActorContext records the authenticated identity initiating an audit event.
type ActorContext struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	Name       string `json:"name,omitempty"`
	ClientIP   string `json:"client_ip"`
	UserAgent  string `json:"user_agent,omitempty"`
	AuthMethod string `json:"auth_method"` // "jwt", "saml", "cli_key", "system"
}

// TargetContext records the organization, repository, or entity being modified.
type TargetContext struct {
	WorkspaceID    uuid.UUID  `json:"workspace_id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	RepositoryID   *uuid.UUID `json:"repository_id,omitempty"`
	RepositoryName string     `json:"repository_name,omitempty"`
	TeamID         *uuid.UUID `json:"team_id,omitempty"`
	TargetEntityID string     `json:"target_entity_id"`
	TargetType     string     `json:"target_type"`
}

// FieldChange records a before-and-after diff for a single modified configuration field.
type FieldChange struct {
	Field    string `json:"field"`
	OldValue any    `json:"old_value"`
	NewValue any    `json:"new_value"`
}

// EnterpriseLogEvent is the standard audit log envelope.
type EnterpriseLogEvent struct {
	ID        uuid.UUID          `json:"id"`
	Category  AuditEventCategory `json:"category"`
	Action    string             `json:"action"` // "CREATE", "UPDATE", "DELETE", "REVOKE", etc.
	Actor     ActorContext       `json:"actor"`
	Target    TargetContext      `json:"target"`
	Changes   []FieldChange      `json:"changes,omitempty"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
	PrevHash  string             `json:"prev_hash,omitempty"`
	Hash      string             `json:"hash,omitempty"`
}

// ═══════════════════════════════════════════════════════════════
// SPECIFIC EVENT PAYLOADS (for each of the 13 categories)
// ═══════════════════════════════════════════════════════════════

// CodeReviewConfigPayload carries code review parameter updates.
type CodeReviewConfigPayload struct {
	AutoApprovalRules map[string]any `json:"auto_approval_rules,omitempty"`
	SeverityThreshold string         `json:"severity_threshold,omitempty"`
	EnabledCategories []string       `json:"enabled_categories,omitempty"`
	ModelRouting      map[string]any `json:"model_routing,omitempty"`
	MaxTokens         int            `json:"max_tokens,omitempty"`
	MaxTurns          int            `json:"max_turns,omitempty"`
}

// DrixyRulesPayload carries custom rule lifecycle updates.
type DrixyRulesPayload struct {
	RuleID      uuid.UUID `json:"rule_id"`
	RuleName    string    `json:"rule_name"`
	Description string    `json:"description,omitempty"`
	Severity    string    `json:"severity"`
	Scope       string    `json:"scope"`
	PathGlobs   []string  `json:"path_globs,omitempty"`
	Prompt      string    `json:"prompt,omitempty"`
	IsActive    bool      `json:"is_active"`
}

// RepositoriesPayload carries repository tracking and webhook events.
type RepositoriesPayload struct {
	RepositoryID   uuid.UUID `json:"repository_id"`
	RepositoryName string    `json:"repository_name"`
	Platform       string    `json:"platform"`
	DefaultBranch  string    `json:"default_branch"`
	IsMonitored    bool      `json:"is_monitored"`
	WebhookID      string    `json:"webhook_id,omitempty"`
}

// ConfigRemovalPayload carries repository or directory config deletion info.
type ConfigRemovalPayload struct {
	TargetScope    string `json:"target_scope"` // "repository" | "directory"
	Path           string `json:"path"`
	RemovedConfigs any    `json:"removed_configs"`
}

// IntegrationPayload carries SCM and PM connection events.
type IntegrationPayload struct {
	Provider       string   `json:"provider"` // "github", "gitlab", "bitbucket", "azuredevops", "jira", "linear"
	AccountName    string   `json:"account_name"`
	ScopesGranted  []string `json:"scopes_granted,omitempty"`
	ConnectionURL  string   `json:"connection_url,omitempty"`
	InstallationID string   `json:"installation_id,omitempty"`
}

// UserStatusPayload carries account state transitions.
type UserStatusPayload struct {
	TargetUserID string `json:"target_user_id"`
	TargetEmail  string `json:"target_email"`
	OldStatus    string `json:"old_status"`
	NewStatus    string `json:"new_status"` // "ACTIVE", "DEACTIVATED", "SUSPENDED"
	Reason       string `json:"reason,omitempty"`
}

// PRMessagesPayload carries PR review commenting templates and suppression rules.
type PRMessagesPayload struct {
	SummaryTemplate string   `json:"summary_template,omitempty"`
	SuppressRules   []string `json:"suppress_rules,omitempty"`
	CommentStyle    string   `json:"comment_style,omitempty"`
	NotifyOnFixes   bool     `json:"notify_on_fixes"`
}

// UserInvitePayload carries enterprise team member invitations.
type UserInvitePayload struct {
	InviteID    uuid.UUID `json:"invite_id"`
	InviteEmail string    `json:"invite_email"`
	AssignedRole string   `json:"assigned_role"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// UserRoleChangePayload carries RBAC permissions and role modifications.
type UserRoleChangePayload struct {
	TargetUserID string `json:"target_user_id"`
	TargetEmail  string `json:"target_email"`
	OldRole      string `json:"old_role"`
	NewRole      string `json:"new_role"` // "OWNER", "ADMIN", "MEMBER", "BILLING_MANAGER", "VIEWER"
}

// UserRepoAccessPayload carries granular repository authorization assignments.
type UserRepoAccessPayload struct {
	TargetUserID   string      `json:"target_user_id"`
	RepositoryIDs  []uuid.UUID `json:"repository_ids"`
	AccessLevel    string      `json:"access_level"` // "READ", "WRITE", "ADMIN"
}

// OrgSettingsPayload carries enterprise SSO, IP allowlists, and retention settings.
type OrgSettingsPayload struct {
	SAMLConfigured      bool     `json:"saml_configured"`
	AllowedDomains      []string `json:"allowed_domains,omitempty"`
	IPAllowlist         []string `json:"ip_allowlist,omitempty"`
	DataRetentionDays   int      `json:"data_retention_days,omitempty"`
	EnforceMFA          bool     `json:"enforce_mfa"`
	SessionTimeoutMins  int      `json:"session_timeout_mins,omitempty"`
}

// CliKeyPayload carries team CLI token management events.
type CliKeyPayload struct {
	KeyID       uuid.UUID `json:"key_id"`
	KeyName     string    `json:"key_name"`
	KeyPrefix   string    `json:"key_prefix"` // e.g. "scandrix_team_..."
	Scopes      []string  `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}
