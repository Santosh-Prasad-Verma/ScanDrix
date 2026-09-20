// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// AstGraphStatus represents the readiness of a repository's semantic AST call graph.
type AstGraphStatus string

const (
	AstGraphStatusPending  AstGraphStatus = "PENDING"
	AstGraphStatusBuilding AstGraphStatus = "BUILDING"
	AstGraphStatusReady    AstGraphStatus = "READY"
	AstGraphStatusFailed   AstGraphStatus = "FAILED"
	AstGraphStatusDisabled AstGraphStatus = "DISABLED"
)

// MaxIncrementalFilesThreshold is the file limit beyond which an incremental merge triggers a full graph rebuild.
const MaxIncrementalFilesThreshold = 500

// AstRepoMetadata captures repository state needed for graph synchronization.
type AstRepoMetadata struct {
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	FullName       string
	DefaultBranch  string
	AstGraphStatus AstGraphStatus
	Platform       models.SCMProvider
}

// AstRepoLookup defines the interface for repository queries.
type AstRepoLookup interface {
	FindRepository(ctx context.Context, platform models.SCMProvider, externalID string) (*AstRepoMetadata, error)
}

// SCMChangedFilesFetcher retrieves modified files for a pull request from the SCM platform.
type SCMChangedFilesFetcher interface {
	GetPRChangedFiles(ctx context.Context, repoFullName string, prNumber int) ([]string, error)
}

// EnqueueAstGraphUpdateInput carries metadata from a merged PR event.
type EnqueueAstGraphUpdateInput struct {
	PRNumber       int
	RepoExternalID string
	RepoName       string
	Platform       models.SCMProvider
	BaseBranch     string
	NewSHA         string
	WorkspaceID    uuid.UUID
}

// AstGraphMergeResult details the job scheduling decision for an AST graph update.
type AstGraphMergeResult struct {
	Enqueued   bool   `json:"enqueued"`
	Reason     string `json:"reason,omitempty"`
	JobType    string `json:"job_type,omitempty"` // "incremental" or "full-rebuild"
	JobID      string `json:"job_id,omitempty"`
	FilesCount int    `json:"files_count,omitempty"`
}

// AstGraphJobPayload holds serializable job data for RabbitMQ / Outbox workers.
type AstGraphJobPayload struct {
	JobID        string             `json:"job_id"`
	RepositoryID uuid.UUID          `json:"repository_id"`
	WorkspaceID  uuid.UUID          `json:"workspace_id"`
	FullName     string             `json:"full_name"`
	Platform     models.SCMProvider `json:"platform"`
	DefaultBranch string            `json:"default_branch"`
	JobType      string             `json:"job_type"` // "incremental" or "full-rebuild"
	ChangedFiles []string           `json:"changed_files,omitempty"`
	NewSHA       string             `json:"new_sha,omitempty"`
	EnqueuedAt   time.Time          `json:"enqueued_at"`
}

// AstGraphMergeCoordinator orchestrates updating the persisted baseline AST graph when PRs merge.
type AstGraphMergeCoordinator struct {
	repoLookup   AstRepoLookup
	filesFetcher SCMChangedFilesFetcher
	publisher    WorkflowJobPublisher
	logger       *slog.Logger
}

// NewAstGraphMergeCoordinator instantiates an AST graph merge coordinator.
func NewAstGraphMergeCoordinator(
	repoLookup AstRepoLookup,
	filesFetcher SCMChangedFilesFetcher,
	publisher WorkflowJobPublisher,
	logger *slog.Logger,
) *AstGraphMergeCoordinator {
	if logger == nil {
		logger = slog.Default()
	}
	return &AstGraphMergeCoordinator{
		repoLookup:   repoLookup,
		filesFetcher: filesFetcher,
		publisher:    publisher,
		logger:       logger,
	}
}

// Execute checks preconditions and enqueues either an incremental or full rebuild job.
func (c *AstGraphMergeCoordinator) Execute(
	ctx context.Context,
	input EnqueueAstGraphUpdateInput,
) (AstGraphMergeResult, error) {
	// 1. Look up repository
	repo, err := c.repoLookup.FindRepository(ctx, input.Platform, input.RepoExternalID)
	if err != nil {
		c.logger.Warn("Failed to lookup repository for merged PR",
			"repo_external_id", input.RepoExternalID,
			"platform", input.Platform,
			"pr_number", input.PRNumber,
			"error", err,
		)
		return AstGraphMergeResult{Enqueued: false, Reason: "repo lookup failed"}, nil
	}

	if repo == nil {
		return AstGraphMergeResult{Enqueued: false, Reason: "repo not tracked"}, nil
	}

	// 2. Only update graph when merged into default branch
	if input.BaseBranch != repo.DefaultBranch {
		return AstGraphMergeResult{
			Enqueued: false,
			Reason:   fmt.Sprintf("base branch %s is not default (%s)", input.BaseBranch, repo.DefaultBranch),
		}, nil
	}

	// 3. Skip if graph is not currently READY
	if repo.AstGraphStatus != AstGraphStatusReady {
		return AstGraphMergeResult{
			Enqueued: false,
			Reason:   fmt.Sprintf("graph not ready (%s)", repo.AstGraphStatus),
		}, nil
	}

	// 4. Fetch changed files for the merged PR
	changedFiles, err := c.filesFetcher.GetPRChangedFiles(ctx, repo.FullName, input.PRNumber)
	if err != nil {
		c.logger.Warn("Failed to fetch PR files for merged AST graph update",
			"repo_full_name", repo.FullName,
			"pr_number", input.PRNumber,
			"error", err,
		)
		return AstGraphMergeResult{Enqueued: false, Reason: "failed to fetch PR files"}, nil
	}

	if len(changedFiles) == 0 {
		return AstGraphMergeResult{Enqueued: false, Reason: "no changed files"}, nil
	}

	// 5. Select job type: full rebuild if > 500 files, otherwise incremental
	useFullRebuild := len(changedFiles) > MaxIncrementalFilesThreshold
	jobType := "incremental"
	eventType := "workflow.ast_graph.incremental"
	if useFullRebuild {
		jobType = "full-rebuild"
		eventType = "workflow.ast_graph.build"
	}

	jobID := uuid.New().String()
	payload := AstGraphJobPayload{
		JobID:         jobID,
		RepositoryID:  repo.ID,
		WorkspaceID:   input.WorkspaceID,
		FullName:      repo.FullName,
		Platform:      repo.Platform,
		DefaultBranch: repo.DefaultBranch,
		JobType:       jobType,
		ChangedFiles:  changedFiles,
		NewSHA:        input.NewSHA,
		EnqueuedAt:    time.Now().UTC(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return AstGraphMergeResult{Enqueued: false, Reason: "payload marshal error"}, err
	}

	// 6. Enqueue via job publisher
	if err := c.publisher.PublishJob(ctx, eventType, payloadBytes); err != nil {
		c.logger.Error("Failed to enqueue AST graph update job",
			"repo_full_name", repo.FullName,
			"job_type", jobType,
			"error", err,
		)
		return AstGraphMergeResult{Enqueued: false, Reason: "enqueue failed"}, err
	}

	c.logger.Info("Enqueued AST graph update job after PR merge",
		"repo_full_name", repo.FullName,
		"pr_number", input.PRNumber,
		"job_type", jobType,
		"files_count", len(changedFiles),
		"job_id", jobID,
	)

	return AstGraphMergeResult{
		Enqueued:   true,
		JobType:    jobType,
		JobID:      jobID,
		FilesCount: len(changedFiles),
	}, nil
}
