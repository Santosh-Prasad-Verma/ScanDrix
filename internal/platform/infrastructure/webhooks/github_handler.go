package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/comments"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	domainwebhooks "github.com/scandrix/backend/internal/platform/domain/webhooks"
	"github.com/scandrix/backend/pkg/models"
)

// GitHubPullRequestHandler handles incoming GitHub webhooks for PRs and comments.
type GitHubPullRequestHandler struct {
	contextService  *services.WebhookContextService
	reviewEnqueuer  IReviewJobEnqueuer
	outboxRepo      IOutboxRepository
	eventEmitter    IEventEmitter
	chatUseCase     IChatWithDrixyUseCase
	prSaver         IPullRequestSaver
	codeMgmtService contracts.ICodeManagementService
	logger          *slog.Logger
}

// NewGitHubPullRequestHandler initializes a new GitHub webhook event handler.
func NewGitHubPullRequestHandler(
	contextService *services.WebhookContextService,
	reviewEnqueuer IReviewJobEnqueuer,
	outboxRepo IOutboxRepository,
	eventEmitter IEventEmitter,
	chatUseCase IChatWithDrixyUseCase,
	prSaver IPullRequestSaver,
	codeMgmtService contracts.ICodeManagementService,
	logger *slog.Logger,
) *GitHubPullRequestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitHubPullRequestHandler{
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

// CanHandle verifies if the event originates from GitHub and is supported.
func (h *GitHubPullRequestHandler) CanHandle(params contracts.WebhookEventParams) bool {
	if params.PlatformType != models.SCMProviderGitHub {
		return false
	}

	supportedEvents := map[string]bool{
		"pull_request":                 true,
		"issue_comment":                true,
		"pull_request_review_comment": true,
	}

	if !supportedEvents[params.Event] {
		return false
	}

	if params.Event == "pull_request" {
		allowedActions := map[string]bool{
			"opened":           true,
			"synchronize":      true,
			"closed":           true,
			"reopened":         true,
			"ready_for_review": true,
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

// Handle processes incoming GitHub webhook payloads.
func (h *GitHubPullRequestHandler) Handle(ctx context.Context, params contracts.WebhookEventParams) error {
	switch params.Event {
	case "pull_request":
		return h.handlePullRequest(ctx, params)
	case "issue_comment", "pull_request_review_comment":
		return h.handleComment(ctx, params)
	default:
		h.logger.WarnContext(ctx, "Unsupported GitHub event", slog.String("event", params.Event))
		return nil
	}
}

func (h *GitHubPullRequestHandler) handlePullRequest(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload domainwebhooks.GitHubPullRequestPayload
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal GitHub PR payload: %w", err)
	}

	prNumber := payload.Number
	if prNumber == 0 && payload.PullRequest.Number != 0 {
		prNumber = payload.PullRequest.Number
	}
	repoID := fmt.Sprintf("%d", payload.Repository.ID)
	repoName := payload.Repository.Name
	repoFullName := payload.Repository.FullName

	h.logger.InfoContext(ctx, "Processing GitHub pull_request event",
		slog.Int("prNumber", prNumber),
		slog.String("repo", repoFullName),
		slog.String("action", payload.Action),
	)

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderGitHub, repoID, nil)
	if err != nil {
		return fmt.Errorf("failed to resolve webhook context: %w", err)
	}

	if routeCtx == nil {
		h.logger.InfoContext(ctx, "No active automation found for repository, completing processing",
			slog.Int("prNumber", prNumber),
			slog.String("repoId", repoID),
		)
		return nil
	}

	// 1. Handle PR synchronize (commits pushed)
	if payload.Action == "synchronize" {
		if h.reviewEnqueuer != nil {
			_ = h.reviewEnqueuer.EnqueueImplementationCheck(ctx, ImplementationCheckRequest{
				PlatformType:            models.SCMProviderGitHub,
				Event:                   params.Event,
				Trigger:                 payload.Action,
				PullRequestNumber:       prNumber,
				CommitSHA:               payload.After,
				RepositoryID:            repoID,
				RepositoryName:          repoName,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			})
		}

		// Force-push detection heuristic
		if payload.Before != "" && payload.After != "" && payload.Before != payload.After {
			go h.checkForcePush(context.Background(), routeCtx, repoID, repoName, prNumber, payload.Before)
		}
	}

	// 2. Handle PR closed
	if payload.Action == "closed" {
		prKey := fmt.Sprintf("%s:%s:%d", routeCtx.OrganizationAndTeamData.OrganizationID, repoID, prNumber)
		if h.outboxRepo != nil {
			_ = h.outboxRepo.CreateMessage(ctx, "sandbox.events", "sandbox.invalidate", SandboxInvalidatePayload{
				PRKey:  prKey,
				Reason: "pr_closed",
			})
		}

		merged := payload.PullRequest.Merged
		if merged && h.reviewEnqueuer != nil {
			baseRef := payload.PullRequest.Base.Ref
			defaultBranch := payload.Repository.DefaultBranch
			if baseRef == defaultBranch {
				_ = h.reviewEnqueuer.EnqueueAstGraphUpdate(ctx, AstGraphUpdateRequest{
					PlatformType:            models.SCMProviderGitHub,
					PullRequestNumber:       prNumber,
					RepoExternalID:          repoID,
					RepoName:                repoName,
					BaseBranch:              baseRef,
					NewSHA:                  pointerToString(payload.PullRequest.MergeCommitSHA),
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

	// 3. Enqueue Code Review Job for reviewable actions
	if h.reviewEnqueuer != nil {
		jobID, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
			PlatformType:            models.SCMProviderGitHub,
			Event:                   params.Event,
			Action:                  payload.Action,
			Origin:                  "webhook",
			PullRequestNumber:       prNumber,
			RepositoryID:            repoID,
			RepositoryName:          repoName,
			RepositoryFullName:      repoFullName,
			OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			TeamAutomationID:        routeCtx.TeamAutomationID,
			RawPayload:              params.RawPayload,
		})
		if err != nil {
			h.logger.ErrorContext(ctx, "Failed to enqueue code review job",
				slog.Int("prNumber", prNumber),
				slog.Any("error", err),
			)
			return err
		}
		h.logger.InfoContext(ctx, "Code review job enqueued successfully",
			slog.String("jobId", jobID),
			slog.Int("prNumber", prNumber),
		)
	}

	return nil
}

func (h *GitHubPullRequestHandler) handleComment(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload map[string]any
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal comment payload: %w", err)
	}

	action, _ := payload["action"].(string)
	if action == "deleted" {
		return nil
	}

	commentMap, _ := payload["comment"].(map[string]any)
	if commentMap == nil {
		return nil
	}

	commentBody, _ := commentMap["body"].(string)
	commentID := fmt.Sprintf("%v", commentMap["id"])
	if commentBody == "" {
		return nil
	}

	repoMap, _ := payload["repository"].(map[string]any)
	if repoMap == nil {
		return nil
	}
	repoID := fmt.Sprintf("%v", repoMap["id"])
	repoName, _ := repoMap["name"].(string)
	repoFullName, _ := repoMap["full_name"].(string)

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderGitHub, repoID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	botName := routeCtx.BotUsername
	isStart := comments.IsReviewCommand(commentBody, botName)
	isForce := comments.IsForceReviewCommand(commentBody, botName)
	isHeavy := comments.IsHeavyReviewCommand(commentBody, botName)
	hasMarker := comments.HasReviewMarker(commentBody)

	// Determine PR number
	prNumber := 0
	if pr, ok := payload["pull_request"].(map[string]any); ok {
		prNumber = intFromAny(pr["number"])
	} else if issue, ok := payload["issue"].(map[string]any); ok {
		prNumber = intFromAny(issue["number"])
	}

	// 1. @drixy review / start-review command detected
	if isStart && !hasMarker && prNumber > 0 {
		directive := comments.ParseReviewDirective(commentBody, botName)
		origin := "command"
		if isForce {
			origin = "command-force"
		}

		h.logger.InfoContext(ctx, "ScanDrix review command triggered via comment",
			slog.Int("prNumber", prNumber),
			slog.String("origin", origin),
			slog.Bool("heavy", isHeavy),
			slog.String("directive", directive),
		)

		if h.reviewEnqueuer != nil {
			_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
				PlatformType:            models.SCMProviderGitHub,
				Event:                   params.Event,
				Action:                  "synchronize",
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

	// 2. Chat with Drixy from Git comment
	if !hasMarker && !isStart && comments.IsDrixyMentionNonReview(commentBody, botName) && prNumber > 0 {
		if h.chatUseCase != nil {
			h.logger.InfoContext(ctx, "Invoking Git chat with Drixy from comment",
				slog.Int("prNumber", prNumber),
				slog.String("commentId", commentID),
			)
			return h.chatUseCase.Execute(ctx, models.SCMProviderGitHub, repoID, prNumber, commentID, commentBody)
		}
	}

	return nil
}

func (h *GitHubPullRequestHandler) checkForcePush(
	ctx context.Context,
	routeCtx *services.WebhookContextResult,
	repoID, repoName string,
	prNumber int,
	previousHeadSha string,
) {
	if h.codeMgmtService == nil || h.outboxRepo == nil {
		return
	}

	commits, err := h.codeMgmtService.GetCommitsForPullRequestForCodeReview(ctx, types.OrganizationAndTeamData{
		OrganizationID: routeCtx.OrganizationAndTeamData.OrganizationID,
		TeamID:         routeCtx.OrganizationAndTeamData.TeamID,
	}, &types.RepositoryDescriptor{
		ID:   repoID,
		Name: repoName,
	}, prNumber)
	if err != nil {
		return
	}

	stillContainsPreviousHead := false
	for _, c := range commits {
		if c.SHA == previousHeadSha {
			stillContainsPreviousHead = true
			break
		}
	}

	if !stillContainsPreviousHead {
		prKey := fmt.Sprintf("%s:%s:%d", routeCtx.OrganizationAndTeamData.OrganizationID, repoID, prNumber)
		_ = h.outboxRepo.CreateMessage(ctx, "sandbox.events", "sandbox.invalidate", SandboxInvalidatePayload{
			PRKey:  prKey,
			Reason: "force_pushed",
		})
	}
}

func pointerToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intFromAny(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

