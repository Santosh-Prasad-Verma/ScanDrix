// Package dashboard provides use cases for pull request review dashboard analytics, facets, and reporting.
package dashboard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// GetFacetsUseCase orchestrates the computation of dashboard segment counts.
type GetFacetsUseCase struct {
	repo IDashboardExecutionRepository
}

// NewGetFacetsUseCase creates a new facets usecase instance.
func NewGetFacetsUseCase(repo IDashboardExecutionRepository) *GetFacetsUseCase {
	return &GetFacetsUseCase{repo: repo}
}

// Execute retrieves all-time review facets for the specified workspace and team scope.
func (uc *GetFacetsUseCase) Execute(ctx context.Context, query FacetsQuery) (*models.PullRequestsFacets, error) {
	if query.WorkspaceID == uuid.Nil {
		return nil, fmt.Errorf("workspace ID is required")
	}

	if uc.repo == nil {
		return &models.PullRequestsFacets{}, nil
	}

	facets, err := uc.repo.GetPullRequestFacets(ctx, query.WorkspaceID, query.TeamID, query.Scope, query.UserEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pull request facets: %w", err)
	}

	if facets == nil {
		return &models.PullRequestsFacets{}, nil
	}

	return facets, nil
}
