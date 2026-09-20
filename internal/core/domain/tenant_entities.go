package domain

import (
	"time"

	"github.com/google/uuid"
)

// Workspace represents an enterprise tenant boundary isolating repositories, policies, and billing.
type Workspace struct {
	BaseEntity
	Slug                 string      `json:"slug" db:"slug"`
	Name                 string      `json:"name" db:"name"`
	Status               string      `json:"status" db:"status"`
	Tier                 string      `json:"tier" db:"tier"`
	SpendLimitUSD        float64     `json:"spend_limit_usd" db:"spend_limit_usd"`
	CurrentMonthSpendUSD float64     `json:"current_month_spend_usd" db:"current_month_spend_usd"`
	EnforceSpendLimit    bool        `json:"enforce_spend_limit" db:"enforce_spend_limit"`
	TrialEndsAt          *time.Time  `json:"trial_ends_at,omitempty" db:"trial_ends_at"`
	CustomDomain         *string     `json:"custom_domain,omitempty" db:"custom_domain"`
	SAMLRequired         bool        `json:"saml_required" db:"saml_required"`
	AllowedEmailDomains  StringSlice `json:"allowed_email_domains" db:"allowed_email_domains"`
	Metadata             JSONBMap    `json:"metadata" db:"metadata"`
}

// Organization represents an enterprise organization container.
type Organization struct {
	BaseEntity
	WorkspaceID    uuid.UUID `json:"workspace_id" db:"workspace_id"`
	ExternalID     string    `json:"external_id" db:"external_id"`
	Name           string    `json:"name" db:"name"`
	AvatarURL      *string   `json:"avatar_url,omitempty" db:"avatar_url"`
	BillingEmail   string    `json:"billing_email" db:"billing_email"`
	SCMProvider    string    `json:"scm_provider" db:"scm_provider"`
	InstallationID *string   `json:"installation_id,omitempty" db:"installation_id"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	Settings       JSONBMap  `json:"settings" db:"settings"`
}

// OrganizationParameters models tenant-level code review rule overrides and execution toggles.
type OrganizationParameters struct {
	TenantScopedEntity
	OrganizationID       uuid.UUID   `json:"organization_id" db:"organization_id"`
	StrictnessLevel      string      `json:"strictness_level" db:"strictness_level"`
	AutoReviewDrafts     bool        `json:"auto_review_drafts" db:"auto_review_drafts"`
	MinSeverity          string      `json:"min_severity" db:"min_severity"`
	IgnoreDraftPRs       bool        `json:"ignore_draft_prs" db:"ignore_draft_prs"`
	MaxFilesPerReview    int         `json:"max_files_per_review" db:"max_files_per_review"`
	MaxDiffBytes         int         `json:"max_diff_bytes" db:"max_diff_bytes"`
	EnabledRules         StringSlice `json:"enabled_rules" db:"enabled_rules"`
	ExcludedPaths        StringSlice `json:"excluded_paths" db:"excluded_paths"`
	CustomPrompt         *string     `json:"custom_prompt,omitempty" db:"custom_prompt"`
	TelemetryEnabled     bool        `json:"telemetry_enabled" db:"telemetry_enabled"`
	NotificationChannels JSONBMap    `json:"notification_channels" db:"notification_channels"`
}

// GlobalParameters represents system-wide runtime engine parameters and feature flags.
type GlobalParameters struct {
	BaseEntity
	ParamKey        string `json:"param_key" db:"param_key"`
	ParamValue      string `json:"param_value" db:"param_value"`
	ValueType       string `json:"value_type" db:"value_type"`
	Description     string `json:"description" db:"description"`
	IsPublic        bool   `json:"is_public" db:"is_public"`
	RequiresRestart bool   `json:"requires_restart" db:"requires_restart"`
}

// ParametersPreset provides pre-curated rule bundles (e.g. Strict Security, High Velocity, OWASP Top 10).
type ParametersPreset struct {
	BaseEntity
	Slug          string   `json:"slug" db:"slug"`
	Name          string   `json:"name" db:"name"`
	Description   string   `json:"description" db:"description"`
	Category      string   `json:"category" db:"category"`
	Configuration JSONBMap `json:"configuration" db:"configuration"`
	IsDefault     bool     `json:"is_default" db:"is_default"`
}

// Parameters represents repository or team level parameter overrides.
type Parameters struct {
	TenantScopedEntity
	EntityID      uuid.UUID  `json:"entity_id" db:"entity_id"`
	EntityType    string     `json:"entity_type" db:"entity_type"`
	Key           string     `json:"key" db:"key"`
	Value         string     `json:"value" db:"value"`
	DataType      string     `json:"data_type" db:"data_type"`
	InheritedFrom *uuid.UUID `json:"inherited_from,omitempty" db:"inherited_from"`
}

// OrganizationAndTeamDataDto mirrors ScanDrix OrganizationAndTeamDataDto for tenant scoping.
type OrganizationAndTeamDataDto struct {
	TeamID         string `json:"teamId,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"`
}
