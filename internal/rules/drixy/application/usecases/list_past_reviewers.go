// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: list_past_reviewers.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"sort"
	"strings"
)

// PastReviewer identifies a code contributor or reviewer.
type PastReviewer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IPastReviewersProvider fetches team members and PR authors from the code platform.
type IPastReviewersProvider interface {
	GetReviewers(ctx context.Context, organizationID, teamID, repositoryID string, months int) ([]PastReviewer, error)
}

// ListPastReviewersUseCase discovers reviewers eligible for exclusion from rules learning.
type ListPastReviewersUseCase struct {
	provider IPastReviewersProvider
}

// NewListPastReviewersUseCase constructs the use case.
func NewListPastReviewersUseCase(provider IPastReviewersProvider) *ListPastReviewersUseCase {
	return &ListPastReviewersUseCase{provider: provider}
}

// Execute retrieves past reviewers across recent pull requests and repository memberships.
func (uc *ListPastReviewersUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	repositoryID string,
	months int,
) ([]PastReviewer, error) {
	if months <= 0 || months > 12 {
		months = 3
	}

	if uc.provider == nil {
		return []PastReviewer{}, nil
	}

	reviewers, err := uc.provider.GetReviewers(ctx, organizationID, teamID, repositoryID, months)
	if err != nil {
		return []PastReviewer{}, nil
	}

	sort.Slice(reviewers, func(i, j int) bool {
		return strings.ToLower(reviewers[i].Name) < strings.ToLower(reviewers[j].Name)
	})

	return reviewers, nil
}
