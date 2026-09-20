// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// DrixyRuleScope defines the evaluation boundary of a custom rule.
type DrixyRuleScope string

const (
	ScopeFile        DrixyRuleScope = "file"
	ScopePullRequest DrixyRuleScope = "pull_request"
	ScopeDirectory   DrixyRuleScope = "directory"
	ScopeCommit      DrixyRuleScope = "commit"
)

// DrixyRuleStatus represents lifecycle readiness of a rule.
type DrixyRuleStatus string

const (
	StatusActive        DrixyRuleStatus = "active"
	StatusInactive      DrixyRuleStatus = "inactive"
	StatusDraft         DrixyRuleStatus = "draft"
	StatusPendingReview DrixyRuleStatus = "pending_review"
)

// DrixyRuleOrigin defines the provenance of a rule.
type DrixyRuleOrigin string

const (
	OriginManual      DrixyRuleOrigin = "manual"
	OriginLibrary     DrixyRuleOrigin = "library"
	OriginPastReviews DrixyRuleOrigin = "past_reviews"
	OriginRepoSync    DrixyRuleOrigin = "repo_file_sync"
)

// DetectorType indicates the mechanical evaluation mechanism for zero-LLM checks.
type DetectorType string

const (
	DetectorRegex       DetectorType = "regex"
	DetectorTokenSearch DetectorType = "token_search"
	DetectorMultiline   DetectorType = "multiline"
)

// CompiledRuleDetector holds a compiled deterministic pattern for instant line matching.
type CompiledRuleDetector struct {
	Type             DetectorType   `json:"type"`
	Pattern          string         `json:"pattern"`
	CompiledRegex    *regexp.Regexp `json:"-"`
	NegativePattern  string         `json:"negative_pattern,omitempty"`
	CompiledNegative *regexp.Regexp `json:"-"`
	Flags            string         `json:"flags,omitempty"`
	Reason           string         `json:"reason,omitempty"`
	CompiledBy       string         `json:"compiled_by,omitempty"`
}

// RuleAtom represents an individual atomic invariant extracted from a composite rule.
type RuleAtom struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Invariant string          `json:"invariant"`
	Severity  models.FindingSeverity `json:"severity"`
}

// DrixyRule is the canonical model for enterprise custom review rules.
type DrixyRule struct {
	ID              uuid.UUID             `json:"id"`
	OrgID           uuid.UUID             `json:"org_id"`
	TeamID          *uuid.UUID            `json:"team_id,omitempty"`
	RepoID          string                `json:"repo_id"` // uuid string or "global"
	Slug            string                `json:"slug"`
	Title           string                `json:"title"`
	Description     string                `json:"description"`
	Severity        models.FindingSeverity `json:"severity"`
	Scope           DrixyRuleScope        `json:"scope"`
	PathGlobs       []string              `json:"path_globs,omitempty"`
	LanguageFilters []string              `json:"language_filters,omitempty"`
	Detector        *CompiledRuleDetector `json:"detector,omitempty"`
	Atoms           []RuleAtom            `json:"atoms,omitempty"`
	Summary         string                `json:"summary,omitempty"`
	Status          DrixyRuleStatus       `json:"status"`
	Origin          DrixyRuleOrigin       `json:"origin"`
	Inheritable     bool                  `json:"inheritable"`
	ExcludedRepos   []string              `json:"excluded_repos,omitempty"`
	IncludedRepos   []string              `json:"included_repos,omitempty"`
	RemediationHint string                `json:"remediation_hint,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// ResolvedRuleSet partitions active rules into instant mechanical and deep semantic sets.
type ResolvedRuleSet struct {
	MechanicalRules []*DrixyRule `json:"mechanical_rules"`
	SemanticRules   []*DrixyRule `json:"semantic_rules"`
	TotalActive     int          `json:"total_active"`
	InheritedCount  int          `json:"inherited_count"`
	OverriddenCount int          `json:"overridden_count"`
}
