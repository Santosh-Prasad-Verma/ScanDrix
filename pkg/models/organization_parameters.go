package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OrganizationParametersKey represents known configuration keys for organization parameters.
type OrganizationParametersKey string

const (
	OrgParamCategoryWorkitemTypes         OrganizationParametersKey = "category_workitems_type"
	OrgParamTimezoneConfig                OrganizationParametersKey = "timezone_config"
	OrgParamReviewModeConfig              OrganizationParametersKey = "review_mode_config"
	OrgParamDrixyFineTuningConfig         OrganizationParametersKey = "drixy_fine_tuning_config"
	OrgParamAutoJoinConfig                OrganizationParametersKey = "auto_join_config"
	OrgParamBYOKConfig                    OrganizationParametersKey = "byok_config"
	OrgParamCockpitMetricsVisibility      OrganizationParametersKey = "cockpit_metrics_visibility"
	OrgParamAutoLicenseAssignment         OrganizationParametersKey = "auto_license_assignment"
	OrgParamAutoLicenseAllowedUsers       OrganizationParametersKey = "auto_license_allowed_users"
	OrgParamModelOverrides                OrganizationParametersKey = "model_overrides"
	OrgParamCodeReviewPreset              OrganizationParametersKey = "code_review_preset"
	OrgParamLicenseKey                    OrganizationParametersKey = "license_key"
	OrgParamLicenseAssignedUsers          OrganizationParametersKey = "license_assigned_users"
	OrgParamFirstReviewAt                 OrganizationParametersKey = "first_review_at"
	OrgParamSpendLimitConfig              OrganizationParametersKey = "spend_limit_config"
	OrgParamGlobalRulesSourceRepositories OrganizationParametersKey = "global_rules_source_repositories"
)

// String helpers for OrganizationParametersKey
const (
	OrgParamKeyBYOKConfig               = string(OrgParamBYOKConfig)
	OrgParamKeyCockpitMetricsVisibility = string(OrgParamCockpitMetricsVisibility)
	OrgParamKeyAutoLicenseAssignment    = string(OrgParamAutoLicenseAssignment)
	OrgParamKeyAutoLicenseAllowedUsers  = string(OrgParamAutoLicenseAllowedUsers)
	OrgParamKeyModelOverrides           = string(OrgParamModelOverrides)
	OrgParamKeyTimezoneConfig           = string(OrgParamTimezoneConfig)
	OrgParamKeyAutoJoinConfig           = string(OrgParamAutoJoinConfig)
)

// OrganizationParameter represents an organization-level parameter row.
type OrganizationParameter struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	WorkspaceID uuid.UUID       `json:"workspace_id" db:"workspace_id"`
	ConfigKey   string          `json:"config_key" db:"config_key"`
	ConfigValue json.RawMessage `json:"config_value" db:"config_value"`
	Description string          `json:"description" db:"description"`
	IsActive    bool            `json:"is_active" db:"is_active"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}
