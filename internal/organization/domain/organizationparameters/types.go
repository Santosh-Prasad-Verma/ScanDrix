// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparams

// ParameterKey represents known keys for organization-level configuration.
type ParameterKey string

const (
	KeyCategoryWorkitemTypes         ParameterKey = "category_workitems_type"
	KeyTimezoneConfig                ParameterKey = "timezone_config"
	KeyReviewModeConfig              ParameterKey = "review_mode_config"
	KeyDrixyFineTuningConfig         ParameterKey = "drixy_fine_tuning_config"
	KeyAutoJoinConfig                ParameterKey = "auto_join_config"
	KeyBYOKConfig                    ParameterKey = "byok_config"
	KeyCockpitMetricsVisibility      ParameterKey = "cockpit_metrics_visibility"
	KeyAutoLicenseAssignment         ParameterKey = "auto_license_assignment"
	KeyAutoLicenseAllowedUsers       ParameterKey = "auto_license_allowed_users"
	KeyModelOverrides                ParameterKey = "model_overrides"
	KeyCodeReviewPreset              ParameterKey = "code_review_preset"
	KeyLicenseKey                    ParameterKey = "license_key"
	KeyLicenseAssignedUsers          ParameterKey = "license_assigned_users"
	KeyFirstReviewAt                 ParameterKey = "first_review_at"
	KeySpendLimitConfig              ParameterKey = "spend_limit_config"
	KeyGlobalRulesSourceRepositories ParameterKey = "global_rules_source_repositories"
)

// AutoJoinConfigValue defines automated domain onboarding.
type AutoJoinConfigValue struct {
	Enabled bool     `json:"enabled"`
	Domains []string `json:"domains"`
}

// AutoLicenseAssignmentConfigValue defines auto-seat assignment, bot suppression, and seat revocation.
type AutoLicenseAssignmentConfigValue struct {
	Enabled                bool              `json:"enabled"`
	IgnoredUsers           []string          `json:"ignoredUsers"`
	AllowedUsers           []string          `json:"allowedUsers,omitempty"`
	AutoRevokeRemovedUsers bool              `json:"autoRevokeRemovedUsers,omitempty"`
	RevokeGraceDays        int               `json:"revokeGraceDays,omitempty"`
	PendingRevocations     map[string]string `json:"pendingRevocations,omitempty"`
	SeededBotIDs           []string          `json:"seededBotIds,omitempty"`
	ManuallyAssignedIDs    []string          `json:"manuallyAssignedIds,omitempty"`
}

// CockpitMetricsVisibilityConfigValue governs executive analytics cards.
type CockpitMetricsVisibilityConfigValue struct {
	OverallDORAMetrics  bool `json:"overallDoraMetrics"`
	DeploymentFrequency bool `json:"deploymentFrequency"`
	LeadTimeForChanges  bool `json:"leadTimeForChanges"`
	ChangeFailureRate   bool `json:"changeFailureRate"`
	TimeToRestore       bool `json:"timeToRestore"`
	PRVelocity          bool `json:"prVelocity"`
	PRSizeMetrics       bool `json:"prSizeMetrics"`
	DrixyImpactMetrics  bool `json:"drixyImpactMetrics"`
}

// TimezoneConfigValue defines organization timezone.
type TimezoneConfigValue struct {
	Timezone string `json:"timezone"`
}

// CategoryWorkitemsTypeConfigValue defines issue/work item classification.
type CategoryWorkitemsTypeConfigValue struct {
	Types []string `json:"types"`
}

// ReviewModeConfigValue defines code review operational mode.
type ReviewModeConfigValue struct {
	Mode string `json:"mode"` // default, strict, silent
}

// BYOKCredential represents encrypted provider credentials.
type BYOKCredential struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	APIKey    string `json:"apiKey"` // stored encrypted, returned masked
	BaseURL   string `json:"baseUrl,omitempty"`
	Region    string `json:"region,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Managed   bool   `json:"managed,omitempty"`
}

// BYOKModel represents an enrolled LLM model.
type BYOKModel struct {
	ID           string   `json:"id"`
	ModelID      string   `json:"modelId"`
	Provider     string   `json:"provider"`
	CredentialID string   `json:"credentialId"`
	Capabilities []string `json:"capabilities"`
	IsDefault    bool     `json:"isDefault"`
}

// BYOKConfigValue represents full Bring-Your-Own-Key configuration.
type BYOKConfigValue struct {
	Version     int              `json:"version"`
	Main        *BYOKModel       `json:"main,omitempty"`
	Fallback    *BYOKModel       `json:"fallback,omitempty"`
	Credentials []BYOKCredential `json:"credentials"`
	Models      []BYOKModel      `json:"models"`
}
