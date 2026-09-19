package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/internal/platform/dtos"
)

// IPlatformConfigStore manages team configuration parameters.
type IPlatformConfigStore interface {
	SetFinishOnboard(ctx context.Context, orgID, teamID string) error
	GetTeamAndOrgName(ctx context.Context, teamID string) (teamName, orgName string, err error)
}

// ITrialProvisioner provisions trial enterprise licenses upon onboarding completion.
type ITrialProvisioner interface {
	ProvisionTrial(ctx context.Context, orgID, teamID string) error
}

// IDrixyRulesSyncService manages syncing and generating Drixy review rules.
type IDrixyRulesSyncService interface {
	SyncRepoRules(ctx context.Context, orgID, teamID string) error
	GeneratePastRules(ctx context.Context, orgID, teamID string, months int) error
}

// ITelemetryService dispatches operational metrics and Slack/Discord notifications.
type ITelemetryService interface {
	OnboardingFinished(ctx context.Context, orgID, teamID, userID, userEmail, teamName, orgName string, memberCount int) error
}

// FinishOnboardingUseCase translates finish-onboarding.use-case.ts.
// It seals team onboarding, provisions licensing, detaches Drixy Rules generation,
// triggers the first automated PR review, and emits onboarding telemetry.
type FinishOnboardingUseCase struct {
	configStore     IPlatformConfigStore
	trialProv       ITrialProvisioner
	rulesSync       IDrixyRulesSyncService
	codeReview      *CreatePRCodeReviewUseCase
	codeManagement  contracts.ICodeManagementService
	telemetry       ITelemetryService
}

// NewFinishOnboardingUseCase constructs a new FinishOnboardingUseCase instance.
func NewFinishOnboardingUseCase(
	configStore IPlatformConfigStore,
	trialProv ITrialProvisioner,
	rulesSync IDrixyRulesSyncService,
	codeReview *CreatePRCodeReviewUseCase,
	codeManagement contracts.ICodeManagementService,
	telemetry ITelemetryService,
) *FinishOnboardingUseCase {
	return &FinishOnboardingUseCase{
		configStore:    configStore,
		trialProv:      trialProv,
		rulesSync:      rulesSync,
		codeReview:     codeReview,
		codeManagement: codeManagement,
		telemetry:      telemetry,
	}
}

// Execute marks onboarding as finished and runs the activation sequence.
func (uc *FinishOnboardingUseCase) Execute(
	ctx context.Context,
	orgID, userID, userEmail string,
	params dtos.FinishOnboardingDTO,
) error {
	if strings.TrimSpace(orgID) == "" {
		return fmt.Errorf("organizationId not found in request context")
	}

	if err := params.Validate(); err != nil {
		return fmt.Errorf("invalid onboarding parameters: %w", err)
	}

	teamID := params.TeamID

	// 1. Commit finishOnboard: true in platform configuration
	if uc.configStore != nil {
		if err := uc.configStore.SetFinishOnboard(ctx, orgID, teamID); err != nil {
			return fmt.Errorf("failed to save platform config: %w", err)
		}
	}

	// 2. Provision server-side trial license
	if uc.trialProv != nil {
		if err := uc.trialProv.ProvisionTrial(ctx, orgID, teamID); err != nil {
			slog.WarnContext(ctx, "Best-effort trial provisioning encountered an issue; continuing onboarding",
				"organizationId", orgID,
				"teamId", teamID,
				"error", err,
			)
		}
	}

	// 3. Detached Drixy Rules sync and past-review rule generation
	if uc.rulesSync != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			if err := uc.rulesSync.SyncRepoRules(bgCtx, orgID, teamID); err != nil {
				slog.Error("Background Drixy Rules sync from repo files failed after onboarding",
					"organizationId", orgID,
					"teamId", teamID,
					"error", err,
				)
			}

			if err := uc.rulesSync.GeneratePastRules(bgCtx, orgID, teamID, 3); err != nil {
				slog.Error("Background Drixy Rules generation failed after onboarding",
					"organizationId", orgID,
					"teamId", teamID,
					"error", err,
				)
			}
		}()
	}

	// 4. Initial code review kickoff
	if params.ReviewPR {
		repoID := ""
		if params.RepositoryID != nil {
			repoID = *params.RepositoryID
		}
		repoName := ""
		if params.RepositoryName != nil {
			repoName = *params.RepositoryName
		}
		pullNumber := 0
		if params.PullNumber != nil {
			pullNumber = *params.PullNumber
		}

		if uc.codeReview != nil {
			_, err := uc.codeReview.Execute(ctx, CreatePRCodeReviewParams{
				OrganizationID: orgID,
				TeamID:         teamID,
				RepositoryID:   repoID,
				RepositoryName: repoName,
				PullNumber:     pullNumber,
			})
			if err != nil {
				return fmt.Errorf("failed to kick off review for PR #%d: %w", pullNumber, err)
			}
		}
	}

	// 5. Hydrate telemetry (time-bounded git member count)
	if uc.telemetry != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			teamName := ""
			orgName := ""
			if uc.configStore != nil {
				tn, on, _ := uc.configStore.GetTeamAndOrgName(bgCtx, teamID)
				teamName = tn
				orgName = on
			}

			memberCount := 0
			if uc.codeManagement != nil {
				orgData := types.OrganizationAndTeamData{
					OrganizationID: orgID,
					TeamID:         teamID,
				}
				if members, err := uc.codeManagement.GetListMembers(bgCtx, orgData); err == nil {
					memberCount = len(members)
				}
			}

			_ = uc.telemetry.OnboardingFinished(bgCtx, orgID, teamID, userID, userEmail, teamName, orgName, memberCount)
		}()
	}

	return nil
}
