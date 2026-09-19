package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/comments"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	domainwebhooks "github.com/scandrix/backend/internal/platform/domain/webhooks"
	"github.com/scandrix/backend/pkg/models"
)

// GitLabMergeRequestHandler handles incoming GitLab webhook events.
type GitLabMergeRequestHandler struct {
	contextService  *services.WebhookContextService
	reviewEnqueuer  IReviewJobEnqueuer
	outboxRepo      IOutboxRepository
	eventEmitter    IEventEmitter
	chatUseCase     IChatWithDrixyUseCase
	prSaver         IPullRequestSaver
	codeMgmtService contracts.ICodeManagementService
	logger          *slog.Logger
}

// NewGitLabMergeRequestHandler initializes a new GitLab webhook handler.
func NewGitLabMergeRequestHandler(
	contextService *services.WebhookContextService,
	reviewEnqueuer IReviewJobEnqueuer,
	outboxRepo IOutboxRepository,
	eventEmitter IEventEmitter,
	chatUseCase IChatWithDrixyUseCase,
	prSaver IPullRequestSaver,
	codeMgmtService contracts.ICodeManagementService,
	logger *slog.Logger,
) *GitLabMergeRequestHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitLabMergeRequestHandler{
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

// CanHandle determines if event is from GitLab.
func (h *GitLabMergeRequestHandler) CanHandle(params contracts.WebhookEventParams) bool {
	if params.PlatformType != models.SCMProviderGitLab {
		return false
	}
	return params.Event == "Merge Request Hook" || params.Event == "Note Hook"
}

// Handle processes GitLab merge request and note hooks.
func (h *GitLabMergeRequestHandler) Handle(ctx context.Context, params contracts.WebhookEventParams) error {
	switch params.Event {
	case "Merge Request Hook":
		return h.handleMergeRequest(ctx, params)
	case "Note Hook":
		return h.handleNote(ctx, params)
	default:
		return nil
	}
}

func (h *GitLabMergeRequestHandler) handleMergeRequest(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload domainwebhooks.GitLabMergeRequestPayload
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal GitLab payload: %w", err)
	}

	mr := payload.ObjectAttributes
	prNumber := mr.IID
	repoID := fmt.Sprintf("%d", payload.Project.ID)
	repoName := payload.Project.Name
	repoFullName := payload.Project.PathWithNamespace

	if !shouldTriggerGitLabReview(mr, payload.Changes) {
		h.logger.DebugContext(ctx, "GitLab MR event ignored as non-triggering update",
			slog.Int("iid", prNumber),
			slog.String("action", mr.Action),
		)
		return nil
	}

	disambiguator := extractGitLabHost(payload.Project.WebURL)
	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderGitLab, repoID, disambiguator)
	if err != nil || routeCtx == nil {
		return nil
	}

	// 1. Commit update
	if mr.Action == "update" && mr.LastCommit != nil && mr.OldRev != "" && mr.LastCommit.ID != mr.OldRev {
		if h.reviewEnqueuer != nil {
			_ = h.reviewEnqueuer.EnqueueImplementationCheck(ctx, ImplementationCheckRequest{
				PlatformType:            models.SCMProviderGitLab,
				Event:                   params.Event,
				Trigger:                 mr.Action,
				PullRequestNumber:       prNumber,
				CommitSHA:               mr.LastCommit.ID,
				RepositoryID:            repoID,
				RepositoryName:          repoName,
				OrganizationAndTeamData: routeCtx.OrganizationAndTeamData,
			})
		}
	}

	// 2. MR closed / merged
	if mr.State == "closed" || mr.Action == "close" || mr.State == "merged" || mr.Action == "merge" {
		prKey := fmt.Sprintf("%s:%s:%d", routeCtx.OrganizationAndTeamData.OrganizationID, repoID, prNumber)
		if h.outboxRepo != nil {
			_ = h.outboxRepo.CreateMessage(ctx, "sandbox.events", "sandbox.invalidate", SandboxInvalidatePayload{
				PRKey:  prKey,
				Reason: "pr_closed",
			})
		}

		merged := mr.State == "merged" || mr.Action == "merge"
		if merged && h.reviewEnqueuer != nil && mr.TargetBranch != "" {
			defaultBranch := payload.Project.DefaultBranch
			if mr.TargetBranch == defaultBranch && mr.LastCommit != nil {
				_ = h.reviewEnqueuer.EnqueueAstGraphUpdate(ctx, AstGraphUpdateRequest{
					PlatformType:            models.SCMProviderGitLab,
					PullRequestNumber:       prNumber,
					RepoExternalID:          repoID,
					RepoName:                repoName,
					BaseBranch:              mr.TargetBranch,
					NewSHA:                  mr.LastCommit.ID,
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
			PlatformType:            models.SCMProviderGitLab,
			Event:                   params.Event,
			Action:                  mr.Action,
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

func (h *GitLabMergeRequestHandler) handleNote(ctx context.Context, params contracts.WebhookEventParams) error {
	var payload domainwebhooks.GitLabNotePayload
	if err := json.Unmarshal(params.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal GitLab note: %w", err)
	}

	if payload.MergeRequest == nil {
		return nil // Not an MR comment
	}

	commentBody := payload.ObjectAttributes.Note
	commentID := fmt.Sprintf("%d", payload.ObjectAttributes.ID)
	prNumber := payload.MergeRequest.IID
	repoID := fmt.Sprintf("%d", payload.Project.ID)
	repoName := payload.Project.Name
	repoFullName := payload.Project.PathWithNamespace

	disambiguator := extractGitLabHost(payload.Project.WebURL)
	routeCtx, err := h.contextService.GetContext(ctx, models.SCMProviderGitLab, repoID, disambiguator)
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
				PlatformType:            models.SCMProviderGitLab,
				Event:                   params.Event,
				Action:                  "update",
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

	if !hasMarker && !isStart && comments.IsDrixyMentionNonReview(commentBody, botName) {
		if h.chatUseCase != nil {
			return h.chatUseCase.Execute(ctx, models.SCMProviderGitLab, repoID, prNumber, commentID, commentBody)
		}
	}

	return nil
}

func shouldTriggerGitLabReview(mr domainwebhooks.GitLabMergeRequest, changes *domainwebhooks.GitLabChanges) bool {
	if mr.Action == "open" {
		return true
	}
	if mr.LastCommit != nil && mr.OldRev != "" && mr.LastCommit.ID != mr.OldRev {
		return true
	}
	if mr.State == "merged" || mr.Action == "merge" {
		return true
	}
	if mr.State == "closed" || mr.Action == "close" {
		return true
	}
	if mr.Action == "update" && changes != nil && changes.Draft != nil {
		// Draft toggled to ready
		if changes.Draft.Previous == true && changes.Draft.Current == false {
			return true
		}
	}
	if mr.Action == "update" && changes != nil && changes.Description != nil {
		return false
	}
	return false
}

func extractGitLabHost(webURL string) *services.WebhookDisambiguator {
	if webURL == "" {
		return nil
	}
	parsed, err := url.Parse(webURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(parsed.Hostname())
	return &services.WebhookDisambiguator{Host: host}
}
