package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/comments"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	domainwebhooks "github.com/scandrix/backend/internal/platform/domain/webhooks"
	"github.com/scandrix/backend/pkg/models"
)

// AzureReposPullRequestHandler processes Azure DevOps Git Service Hook webhooks.
type AzureReposPullRequestHandler struct {
	contextService  *services.WebhookContextService
	reviewEnqueuer  IReviewJobEnqueuer
	outboxRepo      IOutboxRepository
	eventEmitter    IEventEmitter
	chatUseCase     IChatWithDrixyUseCase
	prSaver         IPullRequestSaver
	codeMgmtService contracts.ICodeManagementService
	logger          *slog.Logger
	seenMu          sync.Mutex
	seenEvents      map[string]time.Time
}

// NewAzureReposPullRequestHandler initializes a new Azure Repos webhook handler.
func NewAzureReposPullRequestHandler(
	contextService *services.WebhookContextService,
	reviewEnqueuer IReviewJobEnqueuer,
	outboxRepo IOutboxRepository,
	eventEmitter IEventEmitter,
	chatUseCase IChatWithDrixyUseCase,
	prSaver IPullRequestSaver,
	codeMgmtService contracts.ICodeManagementService,
	logger *slog.Logger,
) *AzureReposPullRequestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AzureReposPullRequestHandler{
		contextService:  contextService,
		reviewEnqueuer:  reviewEnqueuer,
		outboxRepo:      outboxRepo,
		eventEmitter:    eventEmitter,
		chatUseCase:     chatUseCase,
		prSaver:         prSaver,
		codeMgmtService: codeMgmtService,
		logger:          logger,
		seenEvents:      make(map[string]time.Time),
	}
}

// CanHandle verifies Azure DevOps Git events.
func (h *AzureReposPullRequestHandler) CanHandle(params contracts.WebhookEventParams) bool {
	if params.PlatformType != models.SCMProviderAzureDevOps {
		return false
	}
	supportedEvents := map[string]bool{
		"git.pullrequest.created":                   true,
		"git.pullrequest.updated":                   true,
		"git.pullrequest.merge.attempted":           true,
		"ms.vss-code.git-pullrequest-comment-event": true,
	}
	return supportedEvents[params.Event]
}

// Handle processes incoming Azure Repos Service Hook payloads.
func (h *AzureReposPullRequestHandler) Handle(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload domainwebhooks.AzureReposWebhookPayload
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal Azure Repos payload: %w", err)
	}

	// Deduplication check
	if payload.ID != "" && h.isDuplicate(payload.ID) {
		h.logger.WarnContext(ctx, "Duplicate Azure Repos event detected, skipping",
			slog.String("eventId", payload.ID),
			slog.String("eventType", params.Event),
		)
		return nil
	}

	switch params.Event {
	case "ms.vss-code.git-pullrequest-comment-event":
		return h.handleComment(ctx, params, payload)
	case "git.pullrequest.created", "git.pullrequest.updated", "git.pullrequest.merge.attempted":
		return h.handlePullRequest(ctx, params, payload)
	default:
		return nil
	}
}

func (h *AzureReposPullRequestHandler) handlePullRequest(
	ctx context.Context,
	params contracts.WebhookEventParams,
	payload domainwebhooks.AzureReposWebhookPayload,
) error {
	pr := payload.Resource.PullRequest
	if pr == nil {
		return nil
	}

	prNumber := pr.PullRequestID
	repoID := pr.Repository.ID
	repoName := pr.Repository.Name

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderAzureDevOps, repoID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	isCompleted := pr.Status == "completed"
	isAbandoned := pr.Status == "abandoned"

	// 1. Commit / PR updated
	if params.Event == "git.pullrequest.updated" && !isCompleted && !isAbandoned {
		if h.reviewEnqueuer != nil && pr.LastMergeSourceCommit != nil {
			_ = h.reviewEnqueuer.EnqueueImplementationCheck(ctx, ImplementationCheckRequest{
				PlatformType:            models.SCMProviderAzureDevOps,
				Event:                   params.Event,
				Trigger:                 params.Event,
				PullRequestNumber:       prNumber,
				CommitSHA:               pr.LastMergeSourceCommit.CommitID,
				RepositoryID:            repoID,
				RepositoryName:          repoName,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			})
		}
	}

	// 2. PR closed / completed
	if isCompleted || isAbandoned {
		prKey := fmt.Sprintf("%s:%s:%d", routeCtx.OrganizationAndTeamData.OrganizationID, repoID, prNumber)
		if h.outboxRepo != nil {
			_ = h.outboxRepo.CreateMessage(ctx, "sandbox.events", "sandbox.invalidate", SandboxInvalidatePayload{
				PRKey:  prKey,
				Reason: "pr_closed",
			})
		}

		if isCompleted && h.reviewEnqueuer != nil && pr.LastMergeCommit != nil {
			baseBranch := pr.TargetRefName
			_ = h.reviewEnqueuer.EnqueueAstGraphUpdate(ctx, AstGraphUpdateRequest{
				PlatformType:            models.SCMProviderAzureDevOps,
				PullRequestNumber:       prNumber,
				RepoExternalID:          repoID,
				RepoName:                repoName,
				BaseBranch:              baseBranch,
				NewSHA:                  pr.LastMergeCommit.CommitID,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			})
		}

		if h.eventEmitter != nil {
			h.eventEmitter.Emit("pull-request.closed", map[string]any{
				"organizationAndTeamData": routeCtx.OrganizationAndTeamData,
				"repositoryId":            repoID,
				"prNumber":                prNumber,
				"merged":                  isCompleted,
			})
		}
		return nil
	}

	// 3. Enqueue Code Review Job
	if h.reviewEnqueuer != nil {
		_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
			PlatformType:            models.SCMProviderAzureDevOps,
			Event:                   params.Event,
			Action:                  pr.Status,
			Origin:                  "webhook",
			PullRequestNumber:       prNumber,
			RepositoryID:            repoID,
			RepositoryName:          repoName,
			RepositoryFullName:      repoName,
			OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			TeamAutomationID:        routeCtx.TeamAutomationID,
			RawPayload:              params.RawPayload,
		})
		return err
	}

	return nil
}

func (h *AzureReposPullRequestHandler) handleComment(
	ctx context.Context,
	params contracts.WebhookEventParams,
	payload domainwebhooks.AzureReposWebhookPayload,
) error {
	comment := payload.Resource.Comment
	pr := payload.Resource.PullRequest
	if comment == nil || pr == nil {
		return nil
	}

	commentBody := comment.Content
	commentID := fmt.Sprintf("%d", comment.ID)
	prNumber := pr.PullRequestID
	repoID := pr.Repository.ID
	repoName := pr.Repository.Name

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderAzureDevOps, repoID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	botName := routeCtx.BotUsername
	isStart := comments.IsReviewCommand(commentBody, botName)
	isForce := comments.IsForceReviewCommand(commentBody, botName)
	isHeavy := comments.IsHeavyReviewCommand(commentBody, botName)
	hasMarker := comments.HasReviewMarker(commentBody)

	if isStart && !hasMarker {
		directive := comments.ParseReviewDirective(commentBody, botName)
		origin := "command"
		if isForce {
			origin = "command-force"
		}

		if h.reviewEnqueuer != nil {
			_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
				PlatformType:            models.SCMProviderAzureDevOps,
				Event:                   params.Event,
				Action:                  "update",
				Origin:                  origin,
				PullRequestNumber:       prNumber,
				RepositoryID:            repoID,
				RepositoryName:          repoName,
				RepositoryFullName:      repoName,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
				TeamAutomationID:        routeCtx.TeamAutomationID,
				ReviewDirective:         directive,
				Heavy:                   isHeavy,
				TriggerCommentID:        commentID,
				RawPayload:              params.RawPayload,
			})
			return err
		}
		return nil
	}

	if !hasMarker && !isStart && comments.IsDrixyMentionNonReview(commentBody, botName) {
		if h.chatUseCase != nil {
			return h.chatUseCase.Execute(ctx, models.SCMProviderAzureDevOps, repoID, prNumber, commentID, commentBody)
		}
	}

	return nil
}

func (h *AzureReposPullRequestHandler) isDuplicate(eventID string) bool {
	h.seenMu.Lock()
	defer h.seenMu.Unlock()

	now := time.Now()
	// Evict entries older than 10 minutes
	for k, v := range h.seenEvents {
		if now.Sub(v) > 10*time.Minute {
			delete(h.seenEvents, k)
		}
	}

	if _, exists := h.seenEvents[eventID]; exists {
		return true
	}
	h.seenEvents[eventID] = now
	return false
}
