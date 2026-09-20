package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/scandrix/backend/internal/issues/domain"
)

// GenerateIssuesFromPRClosedUseCase handles webhook events when a PR is closed or merged.
type GenerateIssuesFromPRClosedUseCase struct {
	logger             *slog.Logger
	managementService  domain.DrixyIssuesManagementService
	pullRequestService domain.PullRequestsIssuesService
}

// NewGenerateIssuesFromPRClosedUseCase creates an initialized use case.
func NewGenerateIssuesFromPRClosedUseCase(
	logger *slog.Logger,
	managementService domain.DrixyIssuesManagementService,
	pullRequestService domain.PullRequestsIssuesService,
) *GenerateIssuesFromPRClosedUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &GenerateIssuesFromPRClosedUseCase{
		logger:             logger,
		managementService:  managementService,
		pullRequestService: pullRequestService,
	}
}

// Execute processes a closed pull request event.
func (uc *GenerateIssuesFromPRClosedUseCase) Execute(ctx context.Context, params domain.ContextToGenerateIssues) error {
	uc.logger.Info("starting issue generation from closed PR",
		"prNumber", params.PullRequest.Number,
		"repo", params.Repository.Name,
		"platform", params.Repository.Platform)

	// Normalize Bitbucket IDs if needed
	if params.Repository.Platform == domain.PlatformBitbucket {
		params.Repository.ID = strings.Trim(params.Repository.ID, "{}")
	}

	// Azure Repos requires status 'completed'
	if params.Repository.Platform == domain.PlatformAzureRepos {
		if strings.ToLower(params.PullRequest.Status) != "completed" {
			uc.logger.Info("skipping issue generation: Azure Repos PR not completed",
				"status", params.PullRequest.Status)
			return nil
		}
	}

	if params.OrganizationAndTeamData.OrganizationID == "" {
		uc.logger.Warn("skipping issue generation: organization ID missing",
			"repo", params.Repository.Name)
		return nil
	}

	// If PR files not provided directly, fetch from service
	if len(params.PRFiles) == 0 && uc.pullRequestService != nil {
		_, files, err := uc.pullRequestService.FindByNumberAndRepositoryName(
			ctx,
			params.PullRequest.Number,
			params.Repository.Name,
			params.OrganizationAndTeamData,
		)
		if err != nil || len(files) == 0 {
			uc.logger.Warn("skipping issue generation: no PR files found in store",
				"prNumber", params.PullRequest.Number,
				"error", err)
			return nil
		}
		params.PRFiles = files
	}

	if len(params.PRFiles) == 0 {
		uc.logger.Warn("skipping issue generation: PR has no files",
			"prNumber", params.PullRequest.Number)
		return nil
	}

	err := uc.managementService.ProcessClosedPR(ctx, params)
	if err != nil {
		uc.logger.Error("error processing closed PR issues",
			"prNumber", params.PullRequest.Number,
			"error", err)
		return fmt.Errorf("failed processing closed PR: %w", err)
	}

	_ = uc.managementService.ClearIssuesCache(ctx, params.OrganizationAndTeamData.OrganizationID)
	return nil
}
