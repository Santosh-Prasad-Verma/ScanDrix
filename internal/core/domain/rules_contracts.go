package domain

import (
	"context"

	"github.com/google/uuid"
)

// DrixyRuleCreateDTO creates or updates an automated code review rule.
type DrixyRuleCreateDTO struct {
	WorkspaceID         uuid.UUID   `json:"workspace_id"`
	RuleKey             string      `json:"rule_key"`
	Name                string      `json:"name"`
	Category            string      `json:"category"`
	Severity            string      `json:"severity"`
	Description         string      `json:"description"`
	Pattern             *string     `json:"pattern,omitempty"`
	PromptInstructions  string      `json:"prompt_instructions"`
	BadExample          *string     `json:"bad_example,omitempty"`
	GoodExample         *string     `json:"good_example,omitempty"`
	ApplicableLanguages StringSlice `json:"applicable_languages"`
	IsActive            bool        `json:"is_active"`
	WeightMultiplier    float32     `json:"weight_multiplier"`
}

// FindLibraryRulesDto mirrors ScanDrix FindLibraryDrixyRulesDto.
type FindLibraryRulesDto struct {
	PaginationDto
	Title       string              `json:"title,omitempty"`
	Severity    string              `json:"severity,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	PlugAndPlay *bool               `json:"plug_and_play,omitempty"`
	Language    ProgrammingLanguage `json:"language,omitempty"`
	Buckets     []string            `json:"buckets,omitempty"`
}

// GenerateRulesDto mirrors ScanDrix GenerateDrixyRulesDTO.
type GenerateRulesDto struct {
	TeamID          string   `json:"teamId"`
	Months          *int     `json:"months,omitempty"`
	Weeks           *int     `json:"weeks,omitempty"`
	Days            *int     `json:"days,omitempty"`
	RepositoriesIDs []string `json:"repositoriesIds,omitempty"`
}

// DrixyRulesRepository contract.
type DrixyRulesRepository interface {
	FindByID(ctx context.Context, wsID, ruleID uuid.UUID) (*DrixyRules, error)
	FindByKey(ctx context.Context, wsID uuid.UUID, ruleKey string) (*DrixyRules, error)
	Create(ctx context.Context, rule *DrixyRules) error
	Update(ctx context.Context, rule *DrixyRules) error
	ListActive(ctx context.Context, wsID uuid.UUID) ([]*DrixyRules, error)
	IncrementLikes(ctx context.Context, wsID, ruleID uuid.UUID) error
}

// SuggestionEmbeddingRepository contract for pgvector semantic search.
type SuggestionEmbeddingRepository interface {
	Store(ctx context.Context, emb *SuggestionEmbedding) error
	FindSimilar(ctx context.Context, wsID, repoID uuid.UUID, vector FloatVector, limit int, threshold float32) ([]*SuggestionEmbedding, error)
}
