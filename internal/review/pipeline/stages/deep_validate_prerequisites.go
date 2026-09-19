package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// ValidationErrorType categorizes permission and licensing failures.
type ValidationErrorType string

const (
	ValidationErrorInvalidLicense   ValidationErrorType = "INVALID_LICENSE"
	ValidationErrorUserNotLicensed  ValidationErrorType = "USER_NOT_LICENSED"
	ValidationErrorBYOKRequired     ValidationErrorType = "BYOK_REQUIRED"
	ValidationErrorPlanLimitReached ValidationErrorType = "PLAN_LIMIT_EXCEEDED"
	ValidationErrorNotError         ValidationErrorType = "NOT_ERROR"
)

// PermissionValidationResult provides the verdict of authorization and seat checks.
type PermissionValidationResult struct {
	Allowed                bool
	ErrorType              ValidationErrorType
	SubscriptionStatus     string // "active", "trial", "expired", "none"
	TrialCreditsExhausted  bool
	BYOKConfigured         bool
	ResolvedSlot           *domain.ResolvedModelSlotInfo
}

// IPermissionValidator abstracts license and BYOK authorization verification.
type IPermissionValidator interface {
	ValidateExecutionPermissions(ctx context.Context, orgID, userGitID string) (PermissionValidationResult, error)
	AutoAssignLicense(ctx context.Context, orgID, userGitID string, prNumber int) (bool, string, error)
	TryHealMissingTrial(ctx context.Context, orgID string) (bool, error)
	UserHoldsLicenseSeat(ctx context.Context, orgID, userGitID string) (bool, error)
}

// IRepositoryExclusionChecker checks if a repo is dedicated to centralized config or global Drixy rules.
type IRepositoryExclusionChecker interface {
	IsCentralizedConfigRepo(ctx context.Context, orgID, repoID string) (bool, error)
	IsGlobalRulesSourceRepo(ctx context.Context, orgID, repoID string) (bool, error)
}

// INotificationRateLimiter throttles user notifications to avoid spam bursts.
type INotificationRateLimiter interface {
	ShouldEmit(ctx context.Context, rateLimitKey string, ttlSeconds int) (bool, error)
	EmitSkippedNoLicense(ctx context.Context, orgID, prURL, repoName, authorUsername string) error
}

// ISCMFeedbackReaction poster for PR/comment reactions and notices.
type ISCMFeedbackReaction interface {
	AddReaction(ctx context.Context, repo models.TrackedRepository, prNumber int, commentID string, reaction string) error
	CreateNoticeComment(ctx context.Context, repo models.TrackedRepository, prNumber int, commentBody string) error
}

// DeepValidatePrerequisitesStage implements Stage 1 canonical gating.
type DeepValidatePrerequisitesStage struct {
	permissionValidator IPermissionValidator
	exclusionChecker    IRepositoryExclusionChecker
	rateLimiter         INotificationRateLimiter
	feedbackReaction    ISCMFeedbackReaction
}

// NewDeepValidatePrerequisitesStage constructs the deep prerequisites validation stage.
func NewDeepValidatePrerequisitesStage(
	pv IPermissionValidator,
	ec IRepositoryExclusionChecker,
	rl INotificationRateLimiter,
	fr ISCMFeedbackReaction,
) *DeepValidatePrerequisitesStage {
	return &DeepValidatePrerequisitesStage{
		permissionValidator: pv,
		exclusionChecker:    ec,
		rateLimiter:         rl,
		feedbackReaction:    fr,
	}
}

func (s *DeepValidatePrerequisitesStage) Name() string {
	return "DeepValidatePrerequisitesStage"
}

func (s *DeepValidatePrerequisitesStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	// Initialize metadata map if absent
	if pCtx.PipelineMetadata == nil {
		pCtx.PipelineMetadata = make(map[string]interface{})
	}

	showStatusFeedback := true
	if pCtx.ResolvedConfig.ShowStatusFeedback {
		showStatusFeedback = pCtx.ResolvedConfig.ShowStatusFeedback
	}
	pCtx.PipelineMetadata["showStatusFeedback"] = showStatusFeedback

	// 1. Basic structural validation
	if pCtx.RepositoryID == [16]byte{} || pCtx.PullNumber <= 0 {
		pCtx.SkipReview = true
		pCtx.SkipReason = "Missing repository or pull request identifier"
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    "Missing required pull request identifiers",
			ReasonCode: "MISSING_DATA",
		}
		return nil
	}

	// 2. PR Lifecycle state: closed, merged, or locked
	isCommand := strings.HasPrefix(pCtx.Origin, "command")
	if (pCtx.PRState == "closed" || pCtx.PRState == "merged") && !isCommand {
		pCtx.SkipReview = true
		pCtx.SkipReason = fmt.Sprintf("Pull request is %s", pCtx.PRState)
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    fmt.Sprintf("Pull request is %s", pCtx.PRState),
			ReasonCode: "PR_CLOSED_OR_MERGED",
		}
		return nil
	}

	if pCtx.IsLocked && !isCommand {
		pCtx.SkipReview = true
		pCtx.SkipReason = "Pull request discussion thread is locked"
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    "Pull request discussion is locked",
			ReasonCode: "PR_LOCKED",
		}
		return nil
	}

	// 3. Automated Title Bypass
	lowerTitle := strings.ToLower(pCtx.Title)
	for _, kw := range pCtx.ResolvedConfig.IgnoredTitleKeywords {
		if kw != "" && strings.Contains(lowerTitle, strings.ToLower(kw)) {
			pCtx.SkipReview = true
			pCtx.SkipReason = fmt.Sprintf("Title matches ignored keyword: '%s'", kw)
			pCtx.StatusInfo = pipeline.PipelineStatusInfo{
				Status:     pipeline.StatusSkipped,
				Message:    fmt.Sprintf("PR title matches ignored keyword: %s", kw),
				ReasonCode: "IGNORED_TITLE_KEYWORD",
			}
			return nil
		}
	}

	orgID := pCtx.WorkspaceID.String()
	repoID := pCtx.RepositoryID.String()

	// 4. Ignored user / bot filtering with license seat override
	if s.permissionValidator != nil && pCtx.Author != "" {
		holdsSeat, err := s.permissionValidator.UserHoldsLicenseSeat(ctx, orgID, pCtx.Author)
		if err == nil && !holdsSeat {
			authorLower := strings.ToLower(pCtx.Author)
			if strings.HasSuffix(authorLower, "[bot]") || authorLower == "dependabot" || authorLower == "renovate" || authorLower == "snyk-bot" {
				pCtx.SkipReview = true
				pCtx.SkipReason = fmt.Sprintf("Automated bot author '%s' does not hold an active seat", pCtx.Author)
				pCtx.StatusInfo = pipeline.PipelineStatusInfo{
					Status:     pipeline.StatusSkipped,
					Message:    "Automated bot user ignored",
					ReasonCode: "USER_IGNORED",
				}
				return nil
			}
		}
	}

	// 5. Centralized config repository review exclusion
	if s.exclusionChecker != nil {
		isCentralized, err := s.exclusionChecker.IsCentralizedConfigRepo(ctx, orgID, repoID)
		if err == nil && isCentralized {
			pCtx.SkipReview = true
			pCtx.SkipReason = "Code reviews are disabled for the centralized configuration repository"
			pCtx.StatusInfo = pipeline.PipelineStatusInfo{
				Status:     pipeline.StatusSkipped,
				Message:    "Centralized config repository excluded from reviews",
				ReasonCode: "CENTRALIZED_CONFIG_REPO",
			}
			return nil
		}

		isGlobalRules, err := s.exclusionChecker.IsGlobalRulesSourceRepo(ctx, orgID, repoID)
		if err == nil && isGlobalRules {
			pCtx.SkipReview = true
			pCtx.SkipReason = "Code reviews are disabled for the global Drixy Rules source repository"
			pCtx.StatusInfo = pipeline.PipelineStatusInfo{
				Status:     pipeline.StatusSkipped,
				Message:    "Global Drixy Rules source repository excluded from reviews",
				ReasonCode: "GLOBAL_RULES_SOURCE_REPO",
			}
			return nil
		}
	}

	// 6. Permission & Licensing Validation
	if s.permissionValidator != nil {
		result, err := s.permissionValidator.ValidateExecutionPermissions(ctx, orgID, pCtx.Author)
		if err == nil && !result.Allowed {
			// Safety net: Try to heal missing trial if onboarded org never got one
			if result.ErrorType == ValidationErrorInvalidLicense {
				healed, healErr := s.permissionValidator.TryHealMissingTrial(ctx, orgID)
				if healErr == nil && healed {
					result, _ = s.permissionValidator.ValidateExecutionPermissions(ctx, orgID, pCtx.Author)
				}
			}

			// If USER_NOT_LICENSED, try auto-assigning an available seat
			if result.ErrorType == ValidationErrorUserNotLicensed {
				assigned, _, assignErr := s.permissionValidator.AutoAssignLicense(ctx, orgID, pCtx.Author, pCtx.PullNumber)
				if assignErr == nil && assigned {
					// Proceed with review
					result.Allowed = true
				}
			}

			if !result.Allowed {
				pCtx.SkipReview = true
				skipReason := s.getLicenseSkipReason(result.ErrorType, result.SubscriptionStatus, result.TrialCreditsExhausted)
				pCtx.SkipReason = skipReason
				pCtx.StatusInfo = pipeline.PipelineStatusInfo{
					Status:     pipeline.StatusSkipped,
					Message:    skipReason,
					ReasonCode: string(result.ErrorType),
				}
				pCtx.PipelineMetadata["notificationHandled"] = true

				// Post provider reaction & notice comment
				if showStatusFeedback && s.feedbackReaction != nil {
					repo := models.TrackedRepository{
						ID:            pCtx.RepositoryID,
						WorkspaceID:   pCtx.WorkspaceID,
						Provider:      pCtx.Provider,
						NamespacePath: pCtx.RepoNamespace,
					}
					reaction := s.getProviderReaction(pCtx.Provider)
					_ = s.feedbackReaction.AddReaction(ctx, repo, pCtx.PullNumber, pCtx.TriggerCommentID, reaction)

					notice := s.buildNoticeMessage(result.ErrorType, result.SubscriptionStatus, result.TrialCreditsExhausted)
					_ = s.feedbackReaction.CreateNoticeComment(ctx, repo, pCtx.PullNumber, notice)
				}

				// Rate-limited author notification
				if s.rateLimiter != nil {
					rateLimitKey := fmt.Sprintf("notif-rate:review_skipped_no_license:%s:%s", pCtx.Author, orgID)
					if allowed, _ := s.rateLimiter.ShouldEmit(ctx, rateLimitKey, 24*3600); allowed {
						prURL := fmt.Sprintf("https://github.com/%s/pull/%d", pCtx.RepoNamespace, pCtx.PullNumber)
						_ = s.rateLimiter.EmitSkippedNoLicense(ctx, orgID, prURL, pCtx.RepoNamespace, pCtx.Author)
					}
				}

				return nil
			}
		}

		if result.ResolvedSlot != nil {
			pCtx.ResolvedConfig.ResolvedModelSlot = result.ResolvedSlot
		}
	}

	return nil
}

func (s *DeepValidatePrerequisitesStage) getLicenseSkipReason(errType ValidationErrorType, subStatus string, trialExhausted bool) string {
	switch errType {
	case ValidationErrorBYOKRequired:
		return "Bring Your Own Key (BYOK) configuration required"
	case ValidationErrorPlanLimitReached:
		if subStatus == "trial" {
			if trialExhausted {
				return "Trial review credits exhausted; connect your own AI key to continue"
			}
			return "Subscription check unavailable; license service temporarily unreachable"
		}
		return "Organization review plan quota limit reached"
	case ValidationErrorUserNotLicensed:
		return "Pull request author does not have an assigned license seat"
	default:
		return "Organization has no active subscription"
	}
}

func (s *DeepValidatePrerequisitesStage) getProviderReaction(provider models.SCMProvider) string {
	switch provider {
	case models.ProviderGitLab:
		return "lock"
	default:
		return "-1" // thumbs_down on GitHub / Forgejo
	}
}

func (s *DeepValidatePrerequisitesStage) buildNoticeMessage(errType ValidationErrorType, subStatus string, trialExhausted bool) string {
	switch errType {
	case ValidationErrorUserNotLicensed:
		return "## User License Not Found 😢\n\n" +
			"To perform automated reviews, ask your workspace admin to assign a seat in [Seat Management](https://app.scandrix.dev/settings/subscription).\n\n" +
			"<!-- drixy-codereview -->"

	case ValidationErrorBYOKRequired:
		return "## BYOK Configuration Required 🔑\n\n" +
			"Your workspace plan requires Bring Your Own Key (BYOK) configuration.\n\n" +
			"Please configure your provider API keys in [AI Providers](https://app.scandrix.dev/byok).\n\n" +
			"<!-- drixy-codereview -->"

	case ValidationErrorPlanLimitReached:
		if subStatus == "trial" {
			if trialExhausted {
				return "## You've used all your free ScanDrix-paid PR reviews 🎁\n\n" +
					"Your trial is still active — this just means the complimentary reviews covered during trial are used up.\n\n" +
					"**[Connect your own AI key](https://app.scandrix.dev/byok)** to keep Drixy reviewing — unlimited reviews on any plan.\n\n" +
					"<!-- drixy-codereview -->"
			}
			return "## Subscription Check Unavailable ⏳\n\n" +
				"We couldn't confirm your subscription right now because the license service is temporarily unreachable.\n\n" +
				"Please re-run the review in a few minutes (or push a new commit) and Drixy will pick it up.\n\n" +
				"<!-- drixy-codereview -->"
		}
		return "## Organization Review Quota Exceeded 📈\n\n" +
			"Your workspace has reached its monthly review quota. Upgrade your plan at [Subscription Settings](https://app.scandrix.dev/settings/subscription).\n\n" +
			"<!-- drixy-codereview -->"

	default:
		return "## Your trial has ended! 😢\n\n" +
			"To keep getting automated reviews, activate your plan at [Subscription Settings](https://app.scandrix.dev/settings/subscription).\n\n" +
			"<!-- drixy-codereview -->"
	}
}
