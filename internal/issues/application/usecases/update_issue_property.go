package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/issues/domain"
)

// UpdateIssuePropertyUseCase updates a single property (severity, label, status) on an issue.
type UpdateIssuePropertyUseCase struct {
	issuesService     domain.IssuesService
	managementService domain.DrixyIssuesManagementService
	authService       domain.IssuesAuthorizationService
	externalTracker   domain.ExternalIssueTracker
}

// NewUpdateIssuePropertyUseCase creates an initialized use case.
func NewUpdateIssuePropertyUseCase(
	issuesService domain.IssuesService,
	managementService domain.DrixyIssuesManagementService,
	authService domain.IssuesAuthorizationService,
	externalTracker domain.ExternalIssueTracker,
) *UpdateIssuePropertyUseCase {
	return &UpdateIssuePropertyUseCase{
		issuesService:     issuesService,
		managementService: managementService,
		authService:       authService,
		externalTracker:   externalTracker,
	}
}

// Execute performs validation, cache invalidation, and property update.
func (uc *UpdateIssuePropertyUseCase) Execute(
	ctx context.Context,
	uuid string,
	field domain.IssuePropertyField,
	value string,
	user domain.UserRef,
) (*domain.Issue, error) {
	issue, err := uc.issuesService.FindByID(ctx, uuid)
	if err != nil || issue == nil || issue.Repository.ID == "" {
		return nil, fmt.Errorf("issue not found: %s", uuid)
	}

	if uc.authService != nil {
		if err := uc.authService.Ensure(ctx, user, "update", "issues", []string{issue.Repository.ID}); err != nil {
			return nil, fmt.Errorf("authorization denied: %w", err)
		}
	}

	if uc.managementService != nil {
		_ = uc.managementService.ClearIssuesCache(ctx, issue.OrganizationID)
	}

	var updated *domain.Issue
	switch field {
	case domain.FieldSeverity:
		updated, err = uc.issuesService.UpdateSeverity(ctx, uuid, domain.SeverityLevel(value))
	case domain.FieldLabel:
		updated, err = uc.issuesService.UpdateLabel(ctx, uuid, value)
	case domain.FieldStatus:
		updated, err = uc.issuesService.UpdateStatus(ctx, uuid, domain.IssueStatus(value))
		if err == nil && uc.externalTracker != nil {
			_ = uc.externalTracker.UpdateIssueStatus(ctx, uuid, domain.IssueStatus(value))
		}
	default:
		return nil, fmt.Errorf("invalid field: %s", field)
	}

	if err != nil {
		return nil, fmt.Errorf("failed updating issue %s: %w", field, err)
	}
	return updated, nil
}
