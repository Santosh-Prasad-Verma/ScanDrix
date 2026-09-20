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

// BitbucketPullRequestHandler processes Bitbucket Cloud and Server/DC webhooks.
type BitbucketPullRequestHandler struct {
	contextService  *services.WebhookContextService
	reviewEnqueuer  IReviewJobEnqueuer
	outboxRepo      IOutboxRepository
	eventEmitter    IEventEmitter
	chatUseCase     IChatWithDrixyUseCase
	prSaver         IPullRequestSaver
	codeMgmtService contracts.ICodeManagementService
	logger          *slog.Logger
}

// NewBitbucketPullRequestHandler initializes a new Bitbucket webhook handler.
func NewBitbucketPullRequestHandler(
	contextService *services.WebhookContextService,
	reviewEnqueuer IReviewJobEnqueuer,
	outboxRepo IOutboxRepository,
	eventEmitter IEventEmitter,
	chatUseCase IChatWithDrixyUseCase,
	prSaver IPullRequestSaver,
	codeMgmtService contracts.ICodeManagementService,
	logger *slog.Logger,
) *BitbucketPullRequestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BitbucketPullRequestHandler{
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

// CanHandle checks if event is from Bitbucket Cloud or Server/DC.
func (h *BitbucketPullRequestHandler) CanHandle(params contracts.WebhookEventParams) bool {
	if params.PlatformType != models.SCMProviderBitbucket {
		return false
	}
	supportedEvents := map[string]bool{
		"pullrequest:created":         true,
		"pullrequest:updated":         true,
		"pullrequest:fulfilled":       true,
		"pullrequest:rejected":        true,
		"pullrequest:comment_created": true,
		"pr:opened":                   true,
		"pr:modified":                 true,
		"pr:reviewer:updated":         true,
		"pr:comment:added":            true,
		"pr:merged":                   true,
		"pr:declined":                 true,
	}
	return supportedEvents[params.Event]
}

// Handle routes Bitbucket webhook events.
func (h *BitbucketPullRequestHandler) Handle(ctx context.Context, params contracts.WebhookEventParams) error {
	switch params.Event {
	case "pullrequest:comment_created", "pr:comment:added":
		return h.handleComment(ctx, params)
	case "pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected",
		"pr:opened", "pr:modified", "pr:merged", "pr:declined":
		return h.handlePullRequest(ctx, params)
	default:
		return nil
	}
}

func (h *BitbucketPullRequestHandler) handlePullRequest(ctx context.Context, params contracts.WebhookEventParams) error {
	var rawPayload map[string]any
	if err := json.Unmarshal(params.RawPayload, &rawPayload); err != nil {
		return fmt.Errorf("failed to parse Bitbucket raw payload: %w", err)
	}

	isDC, _ := rawPayload["isDataCenterEvent"].(bool)
	var repoID, repoName, repoFullName string
	var prNumber int
	var headSHA, baseBranch, defaultBranch string

	if isDC {
		var dcEvent domainwebhooks.WebhookBitbucketDataCenterPullRequestEvent
		if err := json.Unmarshal(params.RawPayload, &dcEvent); err != nil {
			return err
		}
		prNumber = dcEvent.PullRequest.ID
		repoID = fmt.Sprintf("%d", dcEvent.PullRequest.ToRef.Repository.ID)
		repoName = dcEvent.PullRequest.ToRef.Repository.Name
		repoFullName = dcEvent.PullRequest.ToRef.Repository.Slug
		headSHA = dcEvent.PullRequest.FromRef.LatestCommit
		baseBranch = dcEvent.PullRequest.ToRef.DisplayID
		defaultBranch = "master"
	} else {
		var cloudEvent domainwebhooks.WebhookBitbucketPullRequestEvent
		if err := json.Unmarshal(params.RawPayload, &cloudEvent); err != nil {
			return err
		}
		prNumber = cloudEvent.PullRequest.ID
		repoID = domainwebhooks.StripCurlyBracesFromUUID(cloudEvent.Repository.UUID)
		repoName = cloudEvent.Repository.Name
		repoFullName = cloudEvent.Repository.FullName
		headSHA = cloudEvent.PullRequest.Source.Commit.Hash
		baseBranch = cloudEvent.PullRequest.Destination.Branch.Name
		defaultBranch = cloudEvent.PullRequest.Destination.Branch.Name
	}

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderBitbucket, repoID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	isUpdate := params.Event == "pullrequest:updated" || params.Event == "pr:modified"
	isMerged := params.Event == "pullrequest:fulfilled" || params.Event == "pr:merged"
	isClosed := params.Event == "pullrequest:rejected" || params.Event == "pr:declined" || isMerged

	// 1. Commit update
	if isUpdate && h.reviewEnqueuer != nil && headSHA != "" {
		_ = h.reviewEnqueuer.EnqueueImplementationCheck(ctx, ImplementationCheckRequest{
			PlatformType:            models.SCMProviderBitbucket,
			Event:                   params.Event,
			Trigger:                 params.Event,
			PullRequestNumber:       prNumber,
			CommitSHA:               headSHA,
			RepositoryID:            repoID,
			RepositoryName:          repoName,
			OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
		})
	}

	// 2. PR closed / fulfilled
	if isClosed {
		prKey := fmt.Sprintf("%s:%s:%d", routeCtx.OrganizationAndTeamData.OrganizationID, repoID, prNumber)
		if h.outboxRepo != nil {
			_ = h.outboxRepo.CreateMessage(ctx, "sandbox.events", "sandbox.invalidate", SandboxInvalidatePayload{
				PRKey:  prKey,
				Reason: "pr_closed",
			})
		}

		if isMerged && h.reviewEnqueuer != nil && baseBranch == defaultBranch && headSHA != "" {
			_ = h.reviewEnqueuer.EnqueueAstGraphUpdate(ctx, AstGraphUpdateRequest{
				PlatformType:            models.SCMProviderBitbucket,
				PullRequestNumber:       prNumber,
				RepoExternalID:          repoID,
				RepoName:                repoName,
				BaseBranch:              baseBranch,
				NewSHA:                  headSHA,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			})
		}

		if h.eventEmitter != nil {
			h.eventEmitter.Emit("pull-request.closed", map[string]any{
				"organizationAndTeamData": routeCtx.OrganizationAndTeamData,
				"repositoryId":            repoID,
				"prNumber":                prNumber,
				"merged":                  isMerged,
			})
		}
		return nil
	}

	// 3. Enqueue Code Review Job
	if h.reviewEnqueuer != nil {
		_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
			PlatformType:            models.SCMProviderBitbucket,
			Event:                   params.Event,
			Action:                  params.Event,
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

func (h *BitbucketPullRequestHandler) handleComment(ctx context.Context, params contracts.WebhookEventParams) error {
	mapper := domainwebhooks.GetMappedPlatform(models.SCMProviderBitbucket)
	var rawPayload map[string]any
	if err := json.Unmarshal(params.RawPayload, &rawPayload); err != nil {
		return err
	}

	comment := mapper.MapComment(rawPayload)
	pr := mapper.MapPullRequest(rawPayload)
	repo := mapper.MapRepository(rawPayload)

	if comment == nil || pr == nil || repo == nil || comment.Body == "" {
		return nil
	}

	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderBitbucket, repo.ID, nil)
	if err != nil || routeCtx == nil {
		return nil
	}

	botName := routeCtx.BotUsername
	isStart := comments.IsReviewCommand(comment.Body, botName)
	isForce := comments.IsForceReviewCommand(comment.Body, botName)
	isHeavy := comments.IsHeavyReviewCommand(comment.Body, botName)
	hasMarker := comments.HasReviewMarker(comment.Body)

	if isStart && !hasMarker {
		directive := comments.ParseReviewDirective(comment.Body, botName)
		origin := "command"
		if isForce {
			origin = "command-force"
		}

		if h.reviewEnqueuer != nil {
			_, err := h.reviewEnqueuer.EnqueueReviewJob(ctx, ReviewJobRequest{
				PlatformType:            models.SCMProviderBitbucket,
				Event:                   params.Event,
				Action:                  "update",
				Origin:                  origin,
				PullRequestNumber:       pr.Number,
				RepositoryID:            repo.ID,
				RepositoryName:          repo.Name,
				RepositoryFullName:      repo.FullName,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
				TeamAutomationID:        routeCtx.TeamAutomationID,
				ReviewDirective:         directive,
				Heavy:                   isHeavy,
				TriggerCommentID:        comment.ID,
				RawPayload:              params.RawPayload,
			})
			return err
		}
		return nil
	}

	if !hasMarker && !isStart && comments.IsDrixyMentionNonReview(comment.Body, botName) {
		if h.chatUseCase != nil {
			return h.chatUseCase.Execute(ctx, models.SCMProviderBitbucket, repo.ID, pr.Number, comment.ID, comment.Body)
		}
	}

	return nil
}
