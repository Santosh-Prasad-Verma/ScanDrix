// Package dashboard provides use cases for pull request review dashboard analytics, facets, and reporting.
package dashboard

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// GetPullRequestAuthorsUseCase backs autocomplete search for PR authors.
type GetPullRequestAuthorsUseCase struct {
	repo IDashboardExecutionRepository
}

// NewGetPullRequestAuthorsUseCase creates a new author suggestion usecase instance.
func NewGetPullRequestAuthorsUseCase(repo IDashboardExecutionRepository) *GetPullRequestAuthorsUseCase {
	return &GetPullRequestAuthorsUseCase{repo: repo}
}

// Execute retrieves distinct authors matching the query.
func (uc *GetPullRequestAuthorsUseCase) Execute(ctx context.Context, query AuthorsQuery) ([]models.PullRequestAuthorSuggestion, error) {
	if query.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace ID is required")
	}

	if uc.repo == nil {
		return []models.PullRequestAuthorSuggestion{}, nil
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}

	search := strings.TrimSpace(query.Search)
	authors, err := uc.repo.GetPullRequestAuthors(ctx, query.WorkspaceID, query.TeamID, search, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pull request authors: %w", err)
	}

	if len(authors) == 0 {
		return []models.PullRequestAuthorSuggestion{}, nil
	}

	return authors, nil
}
