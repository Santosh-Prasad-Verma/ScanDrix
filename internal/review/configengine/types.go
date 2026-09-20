// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package configengine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// ConfigScope indicates the origin layer of a configuration setting.
type ConfigScope string

const (
	ScopeDefault      ConfigScope = "DEFAULT"
	ScopeOrganization ConfigScope = "ORGANIZATION"
	ScopeTeam         ConfigScope = "TEAM"
	ScopeRepository   ConfigScope = "REPOSITORY"
	ScopeInRepo       ConfigScope = "IN_REPO"
)

// ReviewSensitivity indicates how aggressively ScanDrix reports potential issues.
type ReviewSensitivity string

const (
	SensitivityLenient  ReviewSensitivity = "LENIENT"  // Only High & Critical
	SensitivityStandard ReviewSensitivity = "STANDARD" // Medium, High, Critical
	SensitivityStrict   ReviewSensitivity = "STRICT"   // Low, Medium, High, Critical
	SensitivityPedantic ReviewSensitivity = "PEDANTIC" // Everything including info/style
)

// ReviewMode defines agent deliberation depth and step budgets.
type ReviewMode string

const (
	ModeFast   ReviewMode = "fast"
	ModeNormal ReviewMode = "normal"
	ModeDeep   ReviewMode = "deep"
)

// CascadeConfigEntry records the value, origin scope, and entity ID for a single setting.
type CascadeConfigEntry[T any] struct {
	Value    T           `json:"value"`
	Scope    ConfigScope `json:"scope"`
	SourceID string      `json:"source_id,omitempty"`
}

// InheritanceAuditTrail provides full visibility into which tier contributed each config item.
type InheritanceAuditTrail struct {
	ReviewMode                 CascadeConfigEntry[ReviewMode]        `json:"review_mode"`
	Sensitivity                CascadeConfigEntry[ReviewSensitivity] `json:"sensitivity"`
	MaxCommentsPerReview       CascadeConfigEntry[int]               `json:"max_comments_per_review"`
	CommittableSuggestions     CascadeConfigEntry[bool]              `json:"committable_suggestions"`
	RequireTicketContext       CascadeConfigEntry[bool]              `json:"require_ticket_context"`
	AutoApproveCleanPRs        CascadeConfigEntry[bool]              `json:"auto_approve_clean_prs"`
	EnableTraceDecisions       CascadeConfigEntry[bool]              `json:"enable_trace_decisions"`
	IgnoredFilePatternsSources map[string]ConfigScope                `json:"ignored_file_patterns_sources"`
	BranchFiltersSources       map[string]ConfigScope                `json:"branch_filters_sources"`
	ResolvedAt                 time.Time                             `json:"resolved_at"`
}

// InRepoConfiguration represents the serialized contents of `.scandrix.yml` or `.scandrix/config.json`.
type InRepoConfiguration struct {
	Version                    string                 `json:"version" yaml:"version"`
	Enabled                    *bool                  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	ReviewMode                 *ReviewMode            `json:"review_mode,omitempty" yaml:"review_mode,omitempty"`
	Sensitivity                *ReviewSensitivity     `json:"sensitivity,omitempty" yaml:"sensitivity,omitempty"`
	Strictness                 *domain.ModelStrictness `json:"strictness,omitempty" yaml:"strictness,omitempty"`
	ReviewOptions              *domain.ReviewOptions  `json:"review_options,omitempty" yaml:"review_options,omitempty"`
	MaxCommentsPerReview       *int                   `json:"max_comments_per_review,omitempty" yaml:"max_comments_per_review,omitempty"`
	MaxSuggestions             *int                   `json:"max_suggestions,omitempty" yaml:"max_suggestions,omitempty"`
	ByokModelID                string                 `json:"byok_model_id,omitempty" yaml:"byok_model_id,omitempty"`
	ByokModel                  string                 `json:"byok_model,omitempty" yaml:"byok_model,omitempty"`
	CommittableSuggestions     *bool                  `json:"committable_suggestions,omitempty" yaml:"committable_suggestions,omitempty"`
	RequireTicketContext       *bool                  `json:"require_ticket_context,omitempty" yaml:"require_ticket_context,omitempty"`
	AutoApproveCleanPRs        *bool                  `json:"auto_approve_clean_prs,omitempty" yaml:"auto_approve_clean_prs,omitempty"`
	EnableTraceDecisions       *bool                  `json:"enable_trace_decisions,omitempty" yaml:"enable_trace_decisions,omitempty"`
	IgnoredFilePatterns        []string               `json:"ignored_file_patterns,omitempty" yaml:"ignored_file_patterns,omitempty"`
	ExcludedBranchPatterns     []string               `json:"excluded_branch_patterns,omitempty" yaml:"excluded_branch_patterns,omitempty"`
	IncludedBranchPatterns     []string               `json:"included_branch_patterns,omitempty" yaml:"included_branch_patterns,omitempty"`
	CustomRulesPaths           []string               `json:"custom_rules_paths,omitempty" yaml:"custom_rules_paths,omitempty"`
	ExtendsOrganizationConfig  *bool                  `json:"extends_organization_config,omitempty" yaml:"extends_organization_config,omitempty"`
	Metadata                   map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling supporting both camelCase and snake_case aliases.
func (c *InRepoConfiguration) UnmarshalJSON(data []byte) error {
	type Alias InRepoConfiguration
	aux := &struct {
		*Alias
		AltMaxSuggestions       *int                   `json:"maxSuggestions"`
		AltMaxCommentsPerReview *int                   `json:"maxCommentsPerReview"`
		AltReviewMode           *ReviewMode            `json:"reviewMode"`
		AltStrictness           *domain.ModelStrictness `json:"strictness"`
		AltCommittable          *bool                  `json:"committableSuggestions"`
		AltRequireTicket        *bool                  `json:"requireTicketContext"`
		AltAutoApprove          *bool                  `json:"autoApproveCleanPrs"`
		AltByokModelID          string                 `json:"byokModelId"`
		AltByokModel            string                 `json:"byokModel"`
		AltIgnoredFiles         []string               `json:"ignoredFiles"`
		AltIgnorePaths          []string               `json:"ignorePaths"`
		AltBaseBranches         []string               `json:"baseBranches"`
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if c.MaxSuggestions == nil && aux.AltMaxSuggestions != nil {
		c.MaxSuggestions = aux.AltMaxSuggestions
	}
	if c.MaxCommentsPerReview == nil {
		if c.MaxSuggestions != nil {
			c.MaxCommentsPerReview = c.MaxSuggestions
		} else if aux.AltMaxCommentsPerReview != nil {
			c.MaxCommentsPerReview = aux.AltMaxCommentsPerReview
		}
	}
	if c.MaxSuggestions == nil && c.MaxCommentsPerReview != nil {
		c.MaxSuggestions = c.MaxCommentsPerReview
	}
	if c.ReviewMode == nil && aux.AltReviewMode != nil {
		c.ReviewMode = aux.AltReviewMode
	}
	if c.Strictness == nil && aux.AltStrictness != nil {
		c.Strictness = aux.AltStrictness
	}
	if c.CommittableSuggestions == nil && aux.AltCommittable != nil {
		c.CommittableSuggestions = aux.AltCommittable
	}
	if c.RequireTicketContext == nil && aux.AltRequireTicket != nil {
		c.RequireTicketContext = aux.AltRequireTicket
	}
	if c.AutoApproveCleanPRs == nil && aux.AltAutoApprove != nil {
		c.AutoApproveCleanPRs = aux.AltAutoApprove
	}
	if c.ByokModelID == "" && aux.AltByokModelID != "" {
		c.ByokModelID = aux.AltByokModelID
	}
	if c.ByokModel == "" && aux.AltByokModel != "" {
		c.ByokModel = aux.AltByokModel
	}
	if len(aux.AltIgnoredFiles) > 0 {
		c.IgnoredFilePatterns = append(c.IgnoredFilePatterns, aux.AltIgnoredFiles...)
	}
	if len(aux.AltIgnorePaths) > 0 {
		c.IgnoredFilePatterns = append(c.IgnoredFilePatterns, aux.AltIgnorePaths...)
	}
	if len(aux.AltBaseBranches) > 0 {
		c.IncludedBranchPatterns = append(c.IncludedBranchPatterns, aux.AltBaseBranches...)
	}
	return nil
}

// ScopedConfigLayer represents a configuration payload defined at a specific hierarchy tier.
type ScopedConfigLayer struct {
	Scope    ConfigScope
	EntityID string
	Config   domain.CodeReviewConfig
	InRepo   *InRepoConfiguration
}

// CascadeResolutionResult holds the resolved final CodeReviewConfig and its audit trail.
type CascadeResolutionResult struct {
	Config     domain.CodeReviewConfig `json:"config"`
	AuditTrail InheritanceAuditTrail   `json:"audit_trail"`
	Warnings   []string                `json:"warnings,omitempty"`
}

// IConfigPersistenceProvider abstracts storage access for organization, team, and repository configs.
type IConfigPersistenceProvider interface {
	GetOrganizationConfig(ctx context.Context, orgID uuid.UUID) (*domain.CodeReviewConfig, error)
	GetTeamConfig(ctx context.Context, orgID, teamID uuid.UUID) (*domain.CodeReviewConfig, error)
	GetRepositoryConfig(ctx context.Context, orgID, repoID uuid.UUID) (*domain.CodeReviewConfig, error)
	GetRepositoryTeamID(ctx context.Context, orgID, repoID uuid.UUID) (*uuid.UUID, error)
}
