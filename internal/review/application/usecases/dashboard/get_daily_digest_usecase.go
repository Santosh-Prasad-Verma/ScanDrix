// Package dashboard provides use cases for pull request review dashboard analytics, facets, and reporting.
package dashboard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// GetDailyDigestUseCase aggregates today's pull request review statistics.
type GetDailyDigestUseCase struct {
	repo IDashboardExecutionRepository
}

// NewGetDailyDigestUseCase creates a new daily digest usecase instance.
func NewGetDailyDigestUseCase(repo IDashboardExecutionRepository) *GetDailyDigestUseCase {
	return &GetDailyDigestUseCase{repo: repo}
}

// Execute retrieves today's PR review daily digest.
func (uc *GetDailyDigestUseCase) Execute(ctx context.Context, query DailyDigestQuery) (*models.PullRequestsDailyDigest, error) {
	if query.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace ID is required")
	}

	if uc.repo == nil {
		return &models.PullRequestsDailyDigest{}, nil
	}

	digest, err := uc.repo.GetPullRequestDailyDigest(ctx, query.WorkspaceID, query.TeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve daily digest: %w", err)
	}

	if digest == nil {
		return &models.PullRequestsDailyDigest{}, nil
	}

	return digest, nil
}
