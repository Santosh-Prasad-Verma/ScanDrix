package services

import (
	"context"

	"github.com/scandrix/backend/internal/cockpit/domain"
)

// PullRequestCounter abstracts counting pull requests for data validation.
type PullRequestCounter interface {
	CountPullRequests(ctx context.Context, organizationID string, limit int) (int, error)
}

// CockpitValidationService checks whether sufficient data exists to populate cockpit views.
type CockpitValidationService struct {
	counter PullRequestCounter
}

// NewCockpitValidationService creates an initialized validation service.
func NewCockpitValidationService(counter PullRequestCounter) *CockpitValidationService {
	return &CockpitValidationService{counter: counter}
}

// Validate verifies whether an organization has sufficient analyzed pull requests.
func (s *CockpitValidationService) Validate(ctx context.Context, organizationID string) (domain.CockpitValidation, error) {
	if s.counter == nil {
		return domain.CockpitValidation{HasData: false, PullRequestsCount: 0}, nil
	}

	count, err := s.counter.CountPullRequests(ctx, organizationID, 50)
	if err != nil {
		return domain.CockpitValidation{HasData: false, PullRequestsCount: 0}, err
	}

	return domain.CockpitValidation{
		HasData:           count > 0,
		PullRequestsCount: count,
	}, nil
}
