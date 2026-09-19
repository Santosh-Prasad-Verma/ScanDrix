package domain

import (
	"github.com/google/uuid"
)

// DrixyRules models organization and repository specific review rules.
type DrixyRules struct {
	TenantScopedEntity
	RuleKey             string      `json:"rule_key" db:"rule_key"`
	Name                string      `json:"name" db:"name"`
	Category            string      `json:"category" db:"category"`
	Severity            string      `json:"severity" db:"severity"`
	Description         string      `json:"description" db:"description"`
	Pattern             *string     `json:"pattern,omitempty" db:"pattern"`
	PromptInstructions  string      `json:"prompt_instructions" db:"prompt_instructions"`
	BadExample          *string     `json:"bad_example,omitempty" db:"bad_example"`
	GoodExample         *string     `json:"good_example,omitempty" db:"good_example"`
	ApplicableLanguages StringSlice `json:"applicable_languages" db:"applicable_languages"`
	IsActive            bool        `json:"is_active" db:"is_active"`
	IsSystemDefault     bool        `json:"is_system_default" db:"is_system_default"`
	WeightMultiplier    float32     `json:"weight_multiplier" db:"weight_multiplier"`
	LikesCount          int         `json:"likes_count" db:"likes_count"`
}

// DrixyRulesModel alias for DrixyRules.
type DrixyRulesModel = DrixyRules

// CodeReviewSettingsLog audits changes made to code review configuration parameters.
type CodeReviewSettingsLog struct {
	TenantScopedEntity
	ModifiedByUserID uuid.UUID `json:"modified_by_user_id" db:"modified_by_user_id"`
	ScopeType        string    `json:"scope_type" db:"scope_type"`
	ScopeID          uuid.UUID `json:"scope_id" db:"scope_id"`
	PreviousValues   JSONBMap  `json:"previous_values" db:"previous_values"`
	UpdatedValues    JSONBMap  `json:"updated_values" db:"updated_values"`
	ChangeReason     *string   `json:"change_reason,omitempty" db:"change_reason"`
}

// CodeReviewSettingsLogModel alias.
type CodeReviewSettingsLogModel = CodeReviewSettingsLog
