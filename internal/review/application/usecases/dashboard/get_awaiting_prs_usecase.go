// Package dashboard provides use cases for pull request review dashboard analytics, facets, and reporting.
package dashboard

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// GetAwaitingPullRequestsUseCase retrieves open pull requests queued or awaiting review.
type GetAwaitingPullRequestsUseCase struct {
	repo IDashboardExecutionRepository
}

// NewGetAwaitingPullRequestsUseCase creates a new awaiting pull requests usecase instance.
func NewGetAwaitingPullRequestsUseCase(repo IDashboardExecutionRepository) *GetAwaitingPullRequestsUseCase {
	return &GetAwaitingPullRequestsUseCase{repo: repo}
}

// Execute returns pull requests awaiting review, capped and sorted by descending open date.
func (uc *GetAwaitingPullRequestsUseCase) Execute(ctx context.Context, query AwaitingQuery) ([]models.AwaitingPullRequest, error) {
	if query.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace ID is required")
	}

	if uc.repo == nil {
		return []models.AwaitingPullRequest{}, nil
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}

	prs, err := uc.repo.GetAwaitingPullRequests(ctx, query.WorkspaceID, query.TeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve awaiting pull requests: %w", err)
	}

	if len(prs) == 0 {
		return []models.AwaitingPullRequest{}, nil
	}

	// Sort newest first
	sort.Slice(prs, func(i, j int) bool {
		return prs[i].CreatedAt.After(prs[j].CreatedAt)
	})

	if len(prs) > limit {
		prs = prs[:limit]
	}

	return prs, nil
}
