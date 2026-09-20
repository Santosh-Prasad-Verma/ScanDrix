package usecases

import (
	"context"

	"github.com/scandrix/backend/internal/issues/domain"
)

// GetTotalIssuesUseCase calculates total issue counts matching complex filters with permission enforcement.
type GetTotalIssuesUseCase struct {
	issuesService     domain.IssuesService
	managementService domain.DrixyIssuesManagementService
	authService       domain.IssuesAuthorizationService
}

// NewGetTotalIssuesUseCase creates an initialized use case.
func NewGetTotalIssuesUseCase(
	issuesService domain.IssuesService,
	managementService domain.DrixyIssuesManagementService,
	authService domain.IssuesAuthorizationService,
) *GetTotalIssuesUseCase {
	return &GetTotalIssuesUseCase{
		issuesService:     issuesService,
		managementService: managementService,
		authService:       authService,
	}
}

// Execute counts issues matching criteria.
func (uc *GetTotalIssuesUseCase) Execute(ctx context.Context, filter domain.GetIssuesFilter, user domain.UserRef) (int64, error) {
	if uc.authService != nil {
		assignedRepoIDs, err := uc.authService.GetRepositoryScope(ctx, user, "read", "issues")
		if err == nil && assignedRepoIDs != nil {
			filter.RepositoryIDs = assignedRepoIDs
		}
	}

	queryMap, err := uc.managementService.BuildFilter(ctx, filter)
	if err != nil {
		return 0, err
	}

	return uc.issuesService.Count(ctx, queryMap)
}
