// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package suggestionembedded

import "context"

// SuggestionFilter specifies criteria for querying embedded suggestions.
type SuggestionFilter struct {
	OrganizationID     string `json:"organization_id,omitempty"`
	RepositoryID       string `json:"repository_id,omitempty"`
	RepositoryFullName string `json:"repository_full_name,omitempty"`
	Language           string `json:"language,omitempty"`
	FeedbackType       string `json:"feedback_type,omitempty"`
	Limit              int    `json:"limit,omitempty"`
	Offset             int    `json:"offset,omitempty"`
}

// ISuggestionEmbeddedRepository defines data access methods for embedded suggestions.
type ISuggestionEmbeddedRepository interface {
	Create(ctx context.Context, entity SuggestionEmbedded) (*SuggestionEmbeddedEntity, error)
	Find(ctx context.Context, filter SuggestionFilter) ([]*SuggestionEmbeddedEntity, error)
	FindOne(ctx context.Context, suggestionID string) (*SuggestionEmbeddedEntity, error)
	FindByID(ctx context.Context, uuid string) (*SuggestionEmbeddedEntity, error)
	Update(ctx context.Context, suggestionID string, data SuggestionEmbedded) (*SuggestionEmbeddedEntity, error)
	BulkInsert(ctx context.Context, entities []SuggestionEmbedded) ([]*SuggestionEmbeddedEntity, error)
}
