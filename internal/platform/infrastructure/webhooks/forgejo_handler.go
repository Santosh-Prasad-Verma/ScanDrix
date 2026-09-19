package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/comments"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	domainwebhooks "github.com/scandrix/backend/internal/platform/domain/webhooks"
	"github.com/scandrix/backend/pkg/models"
)

// ForgejoPullRequestHandler processes Forgejo and Gitea webhooks.
type ForgejoPullRequestHandler struct {
	contextService  *services.WebhookContextService
	reviewEnqueuer  IReviewJobEnqueuer
	outboxRepo      IOutboxRepository
	eventEmitter    IEventEmitter
	chatUseCase     IChatWithDrixyUseCase
	prSaver         IPullRequestSaver
	codeMgmtService contracts.ICodeManagementService
	logger          *slog.Logger
}

// NewForgejoPullRequestHandler initializes a new Forgejo webhook handler.
func NewForgejoPullRequestHandler(
	contextService *services.WebhookContextService,
	reviewEnqueuer IReviewJobEnqueuer,
	outboxRepo IOutboxRepository,
	eventEmitter IEventEmitter,
	chatUseCase IChatWithDrixyUseCase,
	prSaver IPullRequestSaver,
	codeMgmtService contracts.ICodeManagementService,
	logger *slog.Logger,
) *ForgejoPullRequestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ForgejoPullRequestHandler{
		contextService:  contextService,
		reviewEnqueuer:  reviewEnqueuer,
		outboxRepo:      outboxRepo,
		eventEmitter:    eventEmitter,
		chatUseCase:     chatUseCase,
		prSaver:         prSaver,
		codeMgmtService: codeMgmtService,
		logger:          logger,
	}
}

// CanHandle checks whether the event is from Forgejo or Gitea.
func (h *ForgejoPullRequestHandler) CanHandle(params contracts.WebhookEventParams) bool {
	if params.PlatformType != models.SCMProviderForgejo {
		return false
	}
	supportedEvents := map[string]bool{
		"pull_request":                true,
		"issue_comment":               true,
		"pull_request_review_comment": true,
		"push":                        true,
	}
	if !supportedEvents[params.Event] {
		return false
	}

	if params.Event == "pull_request" {
		allowedActions := map[string]bool{
			"opened":       true,
			"synchronized": true,
			"closed":       true,
			"reopened":     true,
		}
		var payload map[string]any
		if err := json.Unmarshal(params.RawPayload, &payload); err == nil {
			action, _ := payload["action"].(string)
			return allowedActions[action]
		}
		return false
	}

	return true
}

// Handle routes Forgejo webhook events.
func (h *ForgejoPullRequestHandler) Handle(ctx context.Context, params contracts.WebhookEventParams) error {
	switch params.Event {
	case "pull_request":
		return h.handlePullRequest(ctx, params)
	case "issue_comment", "pull_request_review_comment":
		return h.handleComment(ctx, params)
	default:
		return nil
	}
}

func (h *ForgejoPullRequestHandler) handlePullRequest(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload domainwebhooks.WebhookForgejoPullRequestEvent
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal Forgejo PR payload: %w", err)
	}

	prNumber := payload.Number
	if prNumber == 0 {
		prNumber = payload.PullRequest.Number
	}
	repoID := fmt.Sprintf("%d", payload.Repository.ID)
	repoName := payload.Repository.Name
	repoFullName := payload.Repository.FullName
	action := string(payload.Action)

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderForgejo, repoID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	// 1. Commit sync
	if action == "synchronized" && h.reviewEnqueuer != nil {
		_ = h.reviewEnqueuer.EnqueueImplementationCheck(ctx, ImplementationCheckRequest{
			PlatformType:            models.SCMProviderForgejo,
			Event:                   params.Event,
			Trigger:                 action,
			PullRequestNumber:       prNumber,
			CommitSHA:               payload.PullRequest.Head.SHA,
			RepositoryID:            repoID,
			RepositoryName:          repoName,
			OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
		})
	}

	// 2. PR closed
	if action == "closed" {
		prKey := fmt.Sprintf("%s:%s:%d", routeCtx.OrganizationAndTeamData.OrganizationID, repoID, prNumber)
		if h.outboxRepo != nil {
			_ = h.outboxRepo.CreateMessage(ctx, "sandbox.events", "sandbox.invalidate", SandboxInvalidatePayload{
				PRKey:  prKey,
				Reason: "pr_closed",
			})
		}

		merged := payload.PullRequest.Merged
		if merged && h.reviewEnqueuer != nil {
			baseBranch := payload.PullRequest.Base.Ref
			defaultBranch := payload.Repository.DefaultBranch
			if baseBranch == defaultBranch && payload.PullRequest.MergeCommitSHA != nil {
				_ = h.reviewEnqueuer.EnqueueAstGraphUpdate(ctx, AstGraphUpdateRequest{
					PlatformType:            models.SCMProviderForgejo,
					PullRequestNumber:       prNumber,
					RepoExternalID:          repoID,
					RepoName:                repoName,
					BaseBranch:              baseBranch,
					NewSHA:                  *payload.PullRequest.MergeCommitSHA,
					OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
				})
			}
		}

		if h.eventEmitter != nil {
			h.eventEmitter.Emit("pull-request.closed", map[string]any{
				"organizationAndTeamData": routeCtx.OrganizationAndTeamData,
				"repositoryId":            repoID,
				"prNumber":                prNumber,
				"merged":                  merged,
			})
		}
		return nil
	}

	// 3. Enqueue Code Review Job
	if h.reviewEnqueuer != nil {
		_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
			PlatformType:            models.SCMProviderForgejo,
			Event:                   params.Event,
			Action:                  action,
			Origin:                  "webhook",
			PullRequestNumber:       prNumber,
			RepositoryID:            repoID,
			RepositoryName:          repoName,
			RepositoryFullName:      repoFullName,
			OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			TeamAutomationID:        routeCtx.TeamAutomationID,
			RawPayload:              params.RawPayload,
		})
		return err
	}

	return nil
}

func (h *ForgejoPullRequestHandler) handleComment(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload domainwebhooks.WebhookForgejoIssueCommentEvent
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to parse Forgejo comment payload: %w", err)
	}

	if payload.Action == "deleted" || payload.Comment.Body == "" {
		return nil
	}

	prNumber := 0
	if payload.PullRequest != nil {
		prNumber = payload.PullRequest.Number
	}

	repoID := fmt.Sprintf("%d", payload.Repository.ID)
	repoName := payload.Repository.Name
	repoFullName := payload.Repository.FullName
	commentBody := payload.Comment.Body
	commentID := fmt.Sprintf("%d", payload.Comment.ID)

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderForgejo, repoID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	botName := routeCtx.BotUsername
	isStart := comments.IsReviewCommand(commentBody, botName)
	isForce := comments.IsForceReviewCommand(commentBody, botName)
	isHeavy := comments.IsHeavyReviewCommand(commentBody, botName)
	hasMarker := comments.HasReviewMarker(commentBody)

	if isStart && !hasMarker && prNumber > 0 {
		directive := comments.ParseReviewDirective(commentBody, botName)
		origin := "command"
		if isForce {
			origin = "command-force"
		}

		if h.reviewEnqueuer != nil {
			_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
				PlatformType:            models.SCMProviderForgejo,
				Event:                   params.Event,
				Action:                  "synchronized",
				Origin:                  origin,
				PullRequestNumber:       prNumber,
				RepositoryID:            repoID,
				RepositoryName:          repoName,
				RepositoryFullName:      repoFullName,
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

	if !hasMarker && !isStart && comments.IsDrixyMentionNonReview(commentBody, botName) && prNumber > 0 {
		if h.chatUseCase != nil {
			return h.chatUseCase.Execute(ctx, models.SCMProviderForgejo, repoID, prNumber, commentID, commentBody)
		}
	}

	return nil
}
