// Package dashboard provides use cases for pull request review dashboard analytics, facets, and reporting.
package dashboard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// GetEnrichedPullRequestsUseCase orchestrates paginated enriched PR review execution history.
type GetEnrichedPullRequestsUseCase struct {
	repo IDashboardExecutionRepository
}

// NewGetEnrichedPullRequestsUseCase creates a new enriched pull requests usecase instance.
func NewGetEnrichedPullRequestsUseCase(repo IDashboardExecutionRepository) *GetEnrichedPullRequestsUseCase {
	return &GetEnrichedPullRequestsUseCase{repo: repo}
}

// Execute retrieves paginated enriched PR executions with findings breakdown and duration metrics.
func (uc *GetEnrichedPullRequestsUseCase) Execute(
	ctx context.Context,
	query EnrichedPullRequestsQuery,
) (*models.PaginatedEnrichedPullRequests, error) {
	if query.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace ID is required")
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}

	if uc.repo == nil {
		return &models.PaginatedEnrichedPullRequests{
			Data:       []models.EnrichedPullRequestExecution{},
			Total:      0,
			Page:       page,
			Limit:      limit,
			TotalPages: 0,
		}, nil
	}

	filter := models.PullRequestExecutionFilter{
		TeamID:             query.TeamID,
		RepositoryID:       query.RepositoryID,
		RepositoryName:     query.RepositoryName,
		Author:             query.Author,
		AuthorPolicy:       query.AuthorPolicy,
		Status:             query.Status,
		HasSentSuggestions: query.HasSentSuggestions,
		NeedsAttention:     query.NeedsAttention,
		PullRequestTitle:   query.SearchTitle,
		PullRequestNumber:  query.PullRequestNumber,
		CreatedAtFrom:      query.CreatedAtFrom,
		CreatedAtTo:        query.CreatedAtTo,
		Severity:           query.Severity,
		Category:           query.Category,
		Page:               page,
		Limit:              limit,
	}

	result, err := uc.repo.ListPullRequestExecutions(ctx, query.WorkspaceID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list enriched pull requests: %w", err)
	}

	if result == nil {
		return &models.PaginatedEnrichedPullRequests{
			Data:       []models.EnrichedPullRequestExecution{},
			Total:      0,
			Page:       page,
			Limit:      limit,
			TotalPages: 0,
		}, nil
	}

	return result, nil
}
