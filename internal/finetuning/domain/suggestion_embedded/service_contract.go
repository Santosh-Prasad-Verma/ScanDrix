// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package suggestionembedded

import (
	"context"

	reviewdomain "github.com/scandrix/backend/internal/review/domain"
)

// ISuggestionEmbeddedService defines the high-level domain operations for embedded suggestions.
type ISuggestionEmbeddedService interface {
	ISuggestionEmbeddedRepository

	BulkCreateFromMongoData(
		ctx context.Context,
		suggestions []SuggestionToEmbed,
	) ([]*SuggestionEmbeddedEntity, error)

	FindByLanguage(
		ctx context.Context,
		language string,
	) ([]*SuggestionEmbeddedEntity, error)

	FindByFeedbackType(
		ctx context.Context,
		feedbackType string,
	) ([]*SuggestionEmbeddedEntity, error)

	GetByOrganization(
		ctx context.Context,
		organizationID string,
	) (*SuggestionEmbeddedFeedbacks, error)

	GetByRepositoryAndOrganization(
		ctx context.Context,
		repositoryID string,
		organizationID string,
	) (*SuggestionEmbeddedFeedbacks, error)

	GetByOrganizationWithLanguages(
		ctx context.Context,
		organizationID string,
	) (*SuggestionEmbeddedFeedbacksWithLanguage, error)

	GetByRepositoryAndOrganizationWithLanguages(
		ctx context.Context,
		repositoryID string,
		organizationID string,
	) (*SuggestionEmbeddedFeedbacksWithLanguage, error)

	EmbedSuggestionsForSuggestionToEmbed(
		ctx context.Context,
		codeSuggestions []*reviewdomain.CodeSuggestion,
		organizationID string,
		prNumber int,
		repositoryID string,
		repositoryFullName string,
	) ([]SuggestionToEmbed, error)
}
