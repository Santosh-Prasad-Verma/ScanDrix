// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_service.go
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// UserAuditInfo captures identity context for rule modification history.
type UserAuditInfo struct {
	UserID    string `json:"userId"`
	UserEmail string `json:"userEmail"`
}

// BucketInfo describes a rule category classification.
type BucketInfo struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// RulesCount is the number of library rules that list this bucket. It is
	// counted from the catalog rather than left off, so a client that wants to
	// show a total is not left to invent a zero.
	RulesCount int `json:"rules_count"`
}

// LibraryDrixyRule models a pre-authored catalog rule.
type LibraryDrixyRule struct {
	UUID               string                         `json:"uuid"`
	Title              string                         `json:"title"`
	Rule               string                         `json:"rule"`
	WhyIsThisImportant string                         `json:"why_is_this_important,omitempty"`
	Severity           string                         `json:"severity"`
	BadExample         string                         `json:"bad_example,omitempty"`
	GoodExample        string                         `json:"good_example,omitempty"`
	Examples           []interfaces.DrixyRulesExample `json:"examples,omitempty"`
	Language           string                         `json:"language"`
	Buckets            []string                       `json:"buckets"`
	Scope              string                         `json:"scope"`
	PlugAndPlay        bool                           `json:"plug_and_play"`
	PositiveCount      int                            `json:"positiveCount"`
	NegativeCount      int                            `json:"negativeCount"`
	UserFeedback       *string                        `json:"userFeedback,omitempty"`
}

// MemoryCreationAction details result of storing review memory.
type MemoryCreationAction string

const (
	MemoryActionCreated MemoryCreationAction = "created"
	MemoryActionUpdated MemoryCreationAction = "updated"
	MemoryActionSkipped MemoryCreationAction = "skipped"
)

// CreateOrUpdateMemoryResult outputs memory registration status.
type CreateOrUpdateMemoryResult struct {
	Rule             *interfaces.DrixyRule `json:"rule"`
	Action           MemoryCreationAction  `json:"action"`
	RequiresApproval bool                  `json:"requiresApproval"`
	Link             string                `json:"link"`
}

// IDrixyRulesService abstracts high-level business rules orchestrating persistence, plan limits, and catalog.
type IDrixyRulesService interface {
	IDrixyRulesRepository

	GetLibraryDrixyRules(ctx context.Context, filters map[string]any, userID string) ([]LibraryDrixyRule, error)
	GetLibraryDrixyRulesWithFeedback(ctx context.Context, organizationID string, filters map[string]any, userID string) ([]LibraryDrixyRule, error)
	GetLibraryDrixyRulesBuckets(ctx context.Context) ([]BucketInfo, error)

	FindRulesByDirectory(ctx context.Context, organizationID, repositoryID, directoryID string) ([]interfaces.DrixyRule, error)
	UpdateRulesStatusByFilter(ctx context.Context, organizationID, repositoryID, directoryID string, newStatus interfaces.DrixyRulesStatus) (*entities.DrixyRulesEntity, error)

	DeleteRuleWithLogging(ctx context.Context, organizationID, teamID, ruleID string, userInfo *UserAuditInfo) (bool, error)
	UpdateRuleWithLogging(ctx context.Context, organizationID, teamID string, rule *interfaces.DrixyRule, userInfo *UserAuditInfo) (*interfaces.DrixyRule, error)

	UpdateRuleReferences(ctx context.Context, organizationID, ruleID, contextReferenceID string) (*interfaces.DrixyRule, error)
	UpdateRuleDetector(ctx context.Context, organizationID, ruleID string, detector *interfaces.DrixyRuleDetector) (*interfaces.DrixyRule, error)

	GetRulesLimitStatus(ctx context.Context, organizationID, teamID string) (int, error)
	GetRecommendedRulesBySuggestions(ctx context.Context, organizationID, teamID, repositoryID, language string) ([]LibraryDrixyRule, error)

	CreateOrUpdateMemory(ctx context.Context, organizationID, teamID string, memory *interfaces.DrixyRuleMemory, userInfo *UserAuditInfo) (*CreateOrUpdateMemoryResult, error)
	FindMemories(ctx context.Context, organizationID, teamID string, filters *interfaces.FindMemoriesFilters) ([]interfaces.FindMemoriesResult, error)

	SyncRulesWithPlanLimit(ctx context.Context, organizationID string, maxAllowedRules int) (*entities.DrixyRulesEntity, error)
}
