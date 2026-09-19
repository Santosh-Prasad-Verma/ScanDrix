package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// ReviewJobPayload embodies the message payload for asynchronous review execution.
type ReviewJobPayload struct {
	ReviewID      uuid.UUID          `json:"review_id"`
	WorkspaceID   uuid.UUID          `json:"workspace_id"`
	RepositoryID  uuid.UUID          `json:"repository_id"`
	RepoNamespace string             `json:"repo_namespace"`
	Provider      models.SCMProvider `json:"provider"`
	PullNumber    int                `json:"pull_number"`
	Title         string             `json:"title"`
	HeadSHA       string             `json:"head_sha"`
	BaseSHA       string             `json:"base_sha"`
	Author        string             `json:"author"`
	RawDiff       string             `json:"raw_diff"`
	RetryCount    int                `json:"retry_count"`
}

// IPipelineExecutor abstracts the pipeline execution strategy.
type IPipelineExecutor interface {
	Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error
}

// ReviewJobProcessor coordinates execution of background code review tasks.
type ReviewJobProcessor struct {
	pipelineStrategy IPipelineExecutor
	deferralService  domain.IPrReviewDeferralService
	concurrencyGate  domain.IByokConcurrencyGate
}

// NewReviewJobProcessor constructs a review job processor.
func NewReviewJobProcessor(
	strategy IPipelineExecutor,
	deferral domain.IPrReviewDeferralService,
	gate domain.IByokConcurrencyGate,
) *ReviewJobProcessor {
	return &ReviewJobProcessor{
		pipelineStrategy: strategy,
		deferralService:  deferral,
		concurrencyGate:  gate,
	}
}

// ProcessReviewJob executes a pull request review run with concurrency gates and lifecycle locks.
func (p *ReviewJobProcessor) ProcessReviewJob(ctx context.Context, rawPayload []byte) error {
	var payload ReviewJobPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return fmt.Errorf("invalid review job payload: %w", err)
	}

	repoIDStr := payload.RepositoryID.String()

	// 1. Check PR deferral gate (prevents collision if another run holds the PR)
	if p.deferralService != nil {
		shouldDefer, err := p.deferralService.ShouldDefer(ctx, repoIDStr, payload.PullNumber, payload.HeadSHA)
		if err == nil && shouldDefer {
			return fmt.Errorf("pr #%d is already being reviewed; deferring", payload.PullNumber)
		}

		if err := p.deferralService.MarkStarted(ctx, repoIDStr, payload.PullNumber, payload.HeadSHA); err != nil {
			return fmt.Errorf("failed to acquire PR review lock: %w", err)
		}
		defer func() {
			_ = p.deferralService.MarkCompleted(ctx, repoIDStr, payload.PullNumber, payload.HeadSHA)
		}()
	}

	// 2. Build PipelineContext
	pCtx := &pipeline.PipelineContext{
		ReviewID:      payload.ReviewID,
		WorkspaceID:   payload.WorkspaceID,
		RepositoryID:  payload.RepositoryID,
		RepoNamespace: payload.RepoNamespace,
		Provider:      payload.Provider,
		PullNumber:    payload.PullNumber,
		Title:         payload.Title,
		HeadSHA:       payload.HeadSHA,
		BaseSHA:       payload.BaseSHA,
		Author:        payload.Author,
		RawDiff:       payload.RawDiff,
		ReviewParams: dtos.ReviewParametersDTO{
			MaxDiffLines: 1500,
		},
		StartTime: time.Now().UTC(),
	}

	// 3. Execute canonical 16-stage pipeline
	return p.pipelineStrategy.Execute(ctx, pCtx)
}
