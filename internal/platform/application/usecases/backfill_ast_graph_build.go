package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platform/domain/types"
)

// AstGraphStatus represents the indexing status of a repository code AST.
type AstGraphStatus string

const (
	AstGraphStatusNull     AstGraphStatus = ""
	AstGraphStatusPending  AstGraphStatus = "PENDING"
	AstGraphStatusBuilding AstGraphStatus = "BUILDING"
	AstGraphStatusReady    AstGraphStatus = "READY"
	AstGraphStatusFailed   AstGraphStatus = "FAILED"
)

// BackfillAstGraphBuildInput defines parameters for AST graph backfilling.
type BackfillAstGraphBuildInput struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
	Force          bool   `json:"force"`
	Limit          int    `json:"limit"`
}

// SkippedAstRepo details a repository skipped from enqueuing with rationale.
type SkippedAstRepo struct {
	FullName string `json:"fullName"`
	Reason   string `json:"reason"`
}

// AstRepoError captures per-repository failures during backfill processing.
type AstRepoError struct {
	RepositoryID string `json:"repositoryId"`
	Error        string `json:"error"`
}

// BackfillAstGraphBuildOutput summarizes the outcome of the backfill sweep.
type BackfillAstGraphBuildOutput struct {
	Matched  int              `json:"matched"`
	Skipped  []SkippedAstRepo `json:"skipped"`
	Enqueued int              `json:"enqueued"`
	JobIDs   []string         `json:"jobIds"`
	Errors   []AstRepoError   `json:"errors"`
}

// IRepositoryAstStatusStore queries repository AST graph status.
type IRepositoryAstStatusStore interface {
	GetAstGraphStatus(ctx context.Context, orgID, teamID, repoID string) (AstGraphStatus, error)
}

// BackfillAstGraphBuildUseCase coordinates batch AST graph construction
// across all configured repositories belonging to an organization and team.
type BackfillAstGraphBuildUseCase struct {
	configStore     IRepositoryConfigStore
	astQueueService IAstGraphQueueService
	statusStore     IRepositoryAstStatusStore
	logger          *slog.Logger
}

// NewBackfillAstGraphBuildUseCase constructs a new backfill orchestrator.
func NewBackfillAstGraphBuildUseCase(
	configStore IRepositoryConfigStore,
	astQueueService IAstGraphQueueService,
	statusStore IRepositoryAstStatusStore,
	logger *slog.Logger,
) *BackfillAstGraphBuildUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &BackfillAstGraphBuildUseCase{
		configStore:     configStore,
		astQueueService: astQueueService,
		statusStore:     statusStore,
		logger:          logger,
	}
}

// Execute performs the AST graph backfill dispatch.
func (uc *BackfillAstGraphBuildUseCase) Execute(
	ctx context.Context,
	input BackfillAstGraphBuildInput,
) (*BackfillAstGraphBuildOutput, error) {
	if strings.TrimSpace(input.OrganizationID) == "" {
		return nil, fmt.Errorf("organizationId is required")
	}
	if strings.TrimSpace(input.TeamID) == "" {
		return nil, fmt.Errorf("teamId is required")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	output := &BackfillAstGraphBuildOutput{
		Skipped: make([]SkippedAstRepo, 0),
		JobIDs:  make([]string, 0),
		Errors:  make([]AstRepoError, 0),
	}

	if uc.configStore == nil {
		return output, nil
	}

	repos, err := uc.configStore.GetRepositories(ctx, input.OrganizationID, input.TeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch configured repositories: %w", err)
	}

	selected := make([]*types.Repositories, 0)
	for _, r := range repos {
		if r == nil {
			continue
		}
		if r.Selected {
			selected = append(selected, r)
		}
	}

	output.Matched = len(selected)

	for _, repo := range selected {
		if output.Enqueued >= limit {
			break
		}

		repoID := repo.ID
		fullName := repo.FullName
		if fullName == "" {
			fullName = repo.Name
		}
		if fullName == "" {
			fullName = repoID
		}

		// Check AST status policy
		var status AstGraphStatus
		if uc.statusStore != nil {
			status, err = uc.statusStore.GetAstGraphStatus(ctx, input.OrganizationID, input.TeamID, repoID)
			if err != nil {
				uc.logger.WarnContext(ctx, "Failed to resolve AST status, falling back to PENDING",
					"repo", fullName,
					"error", err,
				)
				status = AstGraphStatusPending
			}
		}

		if status == AstGraphStatusBuilding {
			output.Skipped = append(output.Skipped, SkippedAstRepo{
				FullName: fullName,
				Reason:   "BUILDING (job already in flight)",
			})
			continue
		}

		if status == AstGraphStatusReady && !input.Force {
			output.Skipped = append(output.Skipped, SkippedAstRepo{
				FullName: fullName,
				Reason:   "READY (use force=true to rebuild)",
			})
			continue
		}

		if uc.astQueueService != nil {
			jobID := uuid.New().String()
			defaultBranch := repo.DefaultBranch
			if defaultBranch == "" {
				defaultBranch = "main"
			}

			err = uc.astQueueService.EnqueueAstGraphBuild(
				ctx,
				input.OrganizationID,
				input.TeamID,
				repoID,
				repo.Name,
				repo.HTTPURL,
				defaultBranch,
			)
			if err != nil {
				output.Errors = append(output.Errors, AstRepoError{
					RepositoryID: repoID,
					Error:        err.Error(),
				})
				uc.logger.ErrorContext(ctx, "Failed to enqueue AST graph build",
					"repo", fullName,
					"error", err,
				)
				continue
			}

			output.JobIDs = append(output.JobIDs, jobID)
			output.Enqueued++
			uc.logger.InfoContext(ctx, "Enqueued AST graph build job",
				"repo", fullName,
				"prevStatus", status,
				"jobId", jobID,
			)
		}
	}

	return output, nil
}
