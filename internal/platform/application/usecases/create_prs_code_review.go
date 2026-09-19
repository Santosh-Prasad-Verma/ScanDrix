package usecases

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
)

// IAutomationRecorder records automation execution status for audit and dashboard display.
type IAutomationRecorder interface {
	RecordAutomationExecution(ctx context.Context, teamID string, status string, prNumber int, repoID string, err error) error
}

// CreatePRCodeReviewParams specifies PR coordinates for initiating an on-demand code review.
type CreatePRCodeReviewParams struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
	RepositoryID   string `json:"repositoryId"`
	RepositoryName string `json:"repositoryName"`
	PullNumber     int    `json:"pullNumber"`
}

// CreatePRCodeReviewResult conveys success of the initiated code review.
type CreatePRCodeReviewResult struct {
	Success   bool   `json:"success"`
	CommentID string `json:"commentId,omitempty"`
}

// CreatePRCodeReviewUseCase translates create-prs-code-review.use-case.ts.
// It initiates an automated code review on a pull request by issuing the ScanDrix command comment "@drixy start-review".
type CreatePRCodeReviewUseCase struct {
	codeManagement contracts.ICodeManagementService
	recorder       IAutomationRecorder
}

// NewCreatePRCodeReviewUseCase constructs the use case instance.
func NewCreatePRCodeReviewUseCase(
	codeManagement contracts.ICodeManagementService,
	recorder IAutomationRecorder,
) *CreatePRCodeReviewUseCase {
	return &CreatePRCodeReviewUseCase{
		codeManagement: codeManagement,
		recorder:       recorder,
	}
}

// Execute triggers the review comment and registers the automation execution.
func (uc *CreatePRCodeReviewUseCase) Execute(ctx context.Context, params CreatePRCodeReviewParams) (*CreatePRCodeReviewResult, error) {
	if params.PullNumber <= 0 {
		return nil, fmt.Errorf("valid pullNumber is required")
	}

	orgData := types.OrganizationAndTeamData{
		OrganizationID: params.OrganizationID,
		TeamID:         params.TeamID,
	}

	repo := &types.RepositoryDescriptor{
		ID:   params.RepositoryID,
		Name: params.RepositoryName,
	}

	commandBody := "@drixy start-review"

	slog.InfoContext(ctx, "Initiating on-demand PR code review",
		"organizationId", params.OrganizationID,
		"teamId", params.TeamID,
		"repository", params.RepositoryName,
		"prNumber", params.PullNumber,
	)

	comment, err := uc.codeManagement.CreateSingleIssueComment(ctx, orgData, repo, params.PullNumber, commandBody)
	if err != nil || comment == nil {
		slog.ErrorContext(ctx, "Failed to post review trigger comment to PR",
			"organizationId", params.OrganizationID,
			"teamId", params.TeamID,
			"repository", params.RepositoryName,
			"prNumber", params.PullNumber,
			"error", err,
		)

		if uc.recorder != nil {
			_ = uc.recorder.RecordAutomationExecution(ctx, params.TeamID, "ERROR", params.PullNumber, params.RepositoryID, err)
		}

		return nil, fmt.Errorf("error commenting on PR #%d to start review: %w", params.PullNumber, err)
	}

	if uc.recorder != nil {
		_ = uc.recorder.RecordAutomationExecution(ctx, params.TeamID, "SUCCESS", params.PullNumber, params.RepositoryID, nil)
	}

	return &CreatePRCodeReviewResult{
		Success:   true,
		CommentID: comment.ID,
	}, nil
}
