// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package interfaces

import (
	"strings"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// DrixyRuleProcessingStatus defines the asynchronous lifecycle state of a rule.
type DrixyRuleProcessingStatus string

const (
	DrixyRuleProcessingStatusPending    DrixyRuleProcessingStatus = "pending"
	DrixyRuleProcessingStatusProcessing DrixyRuleProcessingStatus = "processing"
	DrixyRuleProcessingStatusCompleted  DrixyRuleProcessingStatus = "completed"
	DrixyRuleProcessingStatusFailed     DrixyRuleProcessingStatus = "failed"
)

// DrixyRulesStatus represents the state of a rule within an organization.
type DrixyRulesStatus string

const (
	DrixyRulesStatusActive   DrixyRulesStatus = "active"
	DrixyRulesStatusRejected DrixyRulesStatus = "rejected"
	DrixyRulesStatusPending  DrixyRulesStatus = "pending"
	DrixyRulesStatusApplied  DrixyRulesStatus = "applied"
	DrixyRulesStatusDeleted  DrixyRulesStatus = "deleted"
	DrixyRulesStatusPaused   DrixyRulesStatus = "paused"

	StatusActive   = DrixyRulesStatusActive
	StatusRejected = DrixyRulesStatusRejected
	StatusPending  = DrixyRulesStatusPending
	StatusApplied  = DrixyRulesStatusApplied
	StatusDeleted  = DrixyRulesStatusDeleted
	StatusPaused   = DrixyRulesStatusPaused
)

// DrixyRulesOrigin defines where a rule originated.
type DrixyRulesOrigin string

const (
	DrixyRulesOriginManual                 DrixyRulesOrigin = "manual"
	DrixyRulesOriginLibrary                DrixyRulesOrigin = "library"
	DrixyRulesOriginPastReviews            DrixyRulesOrigin = "past_reviews"
	DrixyRulesOriginRepoFileSync           DrixyRulesOrigin = "repo_file_sync"
	DrixyRulesOriginGlobalRepoFileSync     DrixyRulesOrigin = "global_repo_file_sync"
	DrixyRulesOriginOnboardingRepoAnalysis DrixyRulesOrigin = "onboarding_repo_analysis"
	DrixyRulesOriginMCPAgent               DrixyRulesOrigin = "mcp_agent"
	DrixyRulesOriginCLI                    DrixyRulesOrigin = "cli"
)

// DrixyRuleCentralizedStatus defines status within centralized repository configuration.
type DrixyRuleCentralizedStatus string

const (
	DrixyRuleCentralizedStatusSynced        DrixyRuleCentralizedStatus = "synced"
	DrixyRuleCentralizedStatusPendingAdd    DrixyRuleCentralizedStatus = "pending_add"
	DrixyRuleCentralizedStatusPendingEdit   DrixyRuleCentralizedStatus = "pending_edit"
	DrixyRuleCentralizedStatusPendingDelete DrixyRuleCentralizedStatus = "pending_delete"
)

// DrixyRulesScope defines rule target scope.
type DrixyRulesScope string

const (
	DrixyRulesScopePullRequest DrixyRulesScope = "pull-request"
	DrixyRulesScopeFile        DrixyRulesScope = "file"
)

// DrixyRulesType defines rule or memory categorization.
type DrixyRulesType string

const (
	DrixyRulesTypeStandard DrixyRulesType = "standard"
	DrixyRulesTypeMemory   DrixyRulesType = "memory"

	TypeStandard = DrixyRulesTypeStandard
	TypeMemory   = DrixyRulesTypeMemory
)

// DrixyRuleRequestType defines whether a pending request creates a new rule or updates an existing one.
type DrixyRuleRequestType string

const (
	DrixyRuleRequestTypeCreate DrixyRuleRequestType = "create"
	DrixyRuleRequestTypeUpdate DrixyRuleRequestType = "update"
)

// DrixyRuleReferenceSyncError records failures when synchronizing rule references.
type DrixyRuleReferenceSyncError struct {
	FileName       string    `json:"fileName"`
	Message        string    `json:"message"`
	ErrorType      string    `json:"errorType"` // not_found, invalid_path, fetch_error, file_too_large, parsing_error
	AttemptedPaths []string  `json:"attemptedPaths,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// DrixyRuleCentralizedConfig holds centralized PR syncing metadata.
type DrixyRuleCentralizedConfig struct {
	Path   string                     `json:"path"`
	Status DrixyRuleCentralizedStatus `json:"status"`
}

// DrixyRulesExtendedContext holds optional workflow context.
type DrixyRulesExtendedContext struct {
	Todo string `json:"todo"`
}

// DrixyRulesExample provides bad and good snippets for rule verification and compiler gating.
type DrixyRulesExample struct {
	Snippet   string `json:"snippet"`
	IsCorrect bool   `json:"isCorrect"`
}

// DrixyRuleSummary provides structured WHAT/HOW validation bullets for long rules (>1000 chars).
type DrixyRuleSummary struct {
	Content     string    `json:"content"`
	SourceHash  string    `json:"sourceHash"`
	GeneratedAt time.Time `json:"generatedAt"`
	Model       string    `json:"model"`
}

// DrixyRuleAtom represents an atomic requirement decomposed from a compound rule.
type DrixyRuleAtom struct {
	ID            string              `json:"id"`
	Title         string              `json:"title"`
	Spec          string              `json:"spec"`
	Examples      []DrixyRulesExample `json:"examples,omitempty"`
	Detector      *DrixyRuleDetector  `json:"detector,omitempty"`
	DeclineReason string              `json:"declineReason,omitempty"`
}

// DrixyRuleAtoms holds the collection of atomic requirements from a decomposed compound rule.
type DrixyRuleAtoms struct {
	Items       []DrixyRuleAtom `json:"items"`
	SourceHash  string          `json:"sourceHash"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Model       string          `json:"model"`
}

// DrixyRuleDetector represents a compiled T0 deterministic regex pattern for mechanical rules.
type DrixyRuleDetector struct {
	Type       string `json:"type"` // "regex"
	Pattern    string `json:"pattern"`
	Flags      string `json:"flags,omitempty"`
	CompiledBy string `json:"compiledBy,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// DrixyRulesInheritance defines organizational inheritance and scoping rules.
type DrixyRulesInheritance struct {
	Inheritable bool     `json:"inheritable"`
	Exclude     []string `json:"exclude"`
	Include     []string `json:"include"`
}

// DrixyRuleExternalReference defines linked source file references.
type DrixyRuleExternalReference struct {
	FilePath        string              `json:"filePath"`
	OriginalText    string              `json:"originalText,omitempty"`
	LineRange       *LineRange          `json:"lineRange,omitempty"`
	Description     string              `json:"description,omitempty"`
	RepositoryName  string              `json:"repositoryName,omitempty"`
	LastContentHash string              `json:"lastContentHash,omitempty"`
	LastValidatedAt *time.Time          `json:"lastValidatedAt,omitempty"`
	EstimatedTokens int                 `json:"estimatedTokens,omitempty"`
	LastFetchError  *ExternalFetchError `json:"lastFetchError,omitempty"`
}

type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type ExternalFetchError struct {
	Message   string    `json:"message"`
	ErrorType string    `json:"errorType"`
	Timestamp time.Time `json:"timestamp"`
}

// DrixyRule represents an individual code review rule or memory in ScanDrix.
type DrixyRule struct {
	UUID               string                      `json:"uuid,omitempty"`
	Title              string                      `json:"title"`
	Rule               string                      `json:"rule"`
	Path               string                      `json:"path,omitempty"`
	SourcePath         string                      `json:"sourcePath,omitempty"`
	CentralizedConfig  *DrixyRuleCentralizedConfig `json:"centralizedConfig,omitempty"`
	SourceAnchor       string                      `json:"sourceAnchor,omitempty"`
	Status             DrixyRulesStatus            `json:"status"`
	Severity           string                      `json:"severity"`
	Label              string                      `json:"label,omitempty"`
	Type               DrixyRulesType              `json:"type,omitempty"`
	ExtendedContext    *DrixyRulesExtendedContext  `json:"extendedContext,omitempty"`
	Examples           []DrixyRulesExample         `json:"examples,omitempty"`
	Detector           *DrixyRuleDetector          `json:"detector,omitempty"`
	Summary            *DrixyRuleSummary           `json:"summary,omitempty"`
	Atoms              *DrixyRuleAtoms             `json:"atoms,omitempty"`
	RepositoryID       string                      `json:"repositoryId"`
	SourceRepositoryID string                      `json:"sourceRepositoryId,omitempty"`
	LastContentHash    string                      `json:"lastContentHash,omitempty"`
	Origin             DrixyRulesOrigin            `json:"origin,omitempty"`
	CreatedAt          *time.Time                  `json:"createdAt,omitempty"`
	UpdatedAt          *time.Time                  `json:"updatedAt,omitempty"`
	Reason             *string                     `json:"reason,omitempty"`
	Scope              DrixyRulesScope             `json:"scope,omitempty"`
	DirectoryID        string                      `json:"directoryId,omitempty"`
	Inheritance        *DrixyRulesInheritance      `json:"inheritance,omitempty"`
	ContextReferenceID string                      `json:"contextReferenceId,omitempty"`
	RequestType        DrixyRuleRequestType        `json:"requestType,omitempty"`
	TargetRuleUUID     string                      `json:"targetRuleUuid,omitempty"`
	ResolvedAt         *time.Time                  `json:"resolvedAt,omitempty"`
	ResolvedBy         string                      `json:"resolvedBy,omitempty"`
	PinnedSync         bool                        `json:"pinnedSync,omitempty"`
	LockedByPlan       bool                        `json:"lockedByPlan,omitempty"`
}

// DrixyRules aggregates all rules belonging to an organization.
type DrixyRules struct {
	UUID           string      `json:"uuid,omitempty"`
	OrganizationID string      `json:"organizationId"`
	Rules          []DrixyRule `json:"rules"`
	CreatedAt      *time.Time  `json:"createdAt,omitempty"`
	UpdatedAt      *time.Time  `json:"updatedAt,omitempty"`
}

// DrixyRuleMemory represents a memory rule variation.
type DrixyRuleMemory struct {
	UUID               string               `json:"uuid,omitempty"`
	Title              string               `json:"title"`
	Rule               string               `json:"rule"`
	Path               string               `json:"path,omitempty"`
	SourcePath         string               `json:"sourcePath,omitempty"`
	Status             DrixyRulesStatus     `json:"status"`
	RepositoryID       string               `json:"repositoryId"`
	SourceRepositoryID string               `json:"sourceRepositoryId,omitempty"`
	LastContentHash    string               `json:"lastContentHash,omitempty"`
	Origin             DrixyRulesOrigin     `json:"origin,omitempty"`
	CreatedAt          *time.Time           `json:"createdAt,omitempty"`
	UpdatedAt          *time.Time           `json:"updatedAt,omitempty"`
	Reason             *string              `json:"reason,omitempty"`
	DirectoryID        string               `json:"directoryId,omitempty"`
	RequestType        DrixyRuleRequestType `json:"requestType,omitempty"`
	TargetRuleUUID     string               `json:"targetRuleUuid,omitempty"`
	ResolvedAt         *time.Time           `json:"resolvedAt,omitempty"`
	ResolvedBy         string               `json:"resolvedBy,omitempty"`
	PinnedSync         bool                 `json:"pinnedSync,omitempty"`
	LockedByPlan       bool                 `json:"lockedByPlan,omitempty"`
}

// FindMemoriesFilters defines parameters for locating past review memories.
type FindMemoriesFilters struct {
	RepositoryID string   `json:"repositoryId,omitempty"`
	DirectoryID  string   `json:"directoryId,omitempty"`
	Path         string   `json:"path,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// FindMemoriesResult models memory search output.
type FindMemoriesResult struct {
	UUID         string `json:"uuid,omitempty"`
	Title        string `json:"title"`
	Rule         string `json:"rule"`
	RepositoryID string `json:"repositoryId"`
	DirectoryID  string `json:"directoryId,omitempty"`
	Path         string `json:"path,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	Link         string `json:"link"`
}

// DrixyRulesSeverityLevel aliases the standard model finding severity.
type DrixyRulesSeverityLevel = models.FindingSeverity

const (
	DrixyRulesSeverityCritical DrixyRulesSeverityLevel = models.SeverityCritical
	DrixyRulesSeverityHigh     DrixyRulesSeverityLevel = models.SeverityHigh
	DrixyRulesSeverityMedium   DrixyRulesSeverityLevel = models.SeverityMedium
	DrixyRulesSeverityLow      DrixyRulesSeverityLevel = models.SeverityLow
)

// ResolveDrixyRuleSeverityLevel converts text severity into standardized finding severity.
func ResolveDrixyRuleSeverityLevel(rule *DrixyRule) models.FindingSeverity {
	if rule == nil {
		return models.SeverityHigh
	}
	return ResolveDrixyRuleSeverityLevelFromString(rule.Severity)
}

// ResolveDrixyRuleSeverityLevelFromString converts arbitrary text severity into standardized finding severity.
func ResolveDrixyRuleSeverityLevelFromString(severity string) models.FindingSeverity {
	if severity == "" {
		return models.SeverityHigh
	}
	switch strings.ToUpper(severity) {
	case string(models.SeverityCritical):
		return models.SeverityCritical
	case string(models.SeverityHigh):
		return models.SeverityHigh
	case string(models.SeverityMedium):
		return models.SeverityMedium
	case string(models.SeverityLow):
		return models.SeverityLow
	default:
		return models.SeverityHigh
	}
}
