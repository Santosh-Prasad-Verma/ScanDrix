package usecases

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
)

const (
	DefaultRevokeGraceDays = 7
)

// AutoAssignLicenseConfig holds organization parameters for automated seat assignment.
type AutoAssignLicenseConfig struct {
	AutoRevokeRemovedUsers bool              `json:"autoRevokeRemovedUsers"`
	RevokeGraceDays        int               `json:"revokeGraceDays"`
	PendingRevocations     map[string]string `json:"pendingRevocations"` // gitID -> RFC3339 timestamp
	AllowedUsers           []string          `json:"allowedUsers"`
}

// IAutoLicenseParametersStore defines persistence for automated seat assignment policies.
type IAutoLicenseParametersStore interface {
	GetAutoAssignConfig(ctx context.Context, orgID, teamID string) (*AutoAssignLicenseConfig, error)
	SaveAutoAssignConfig(ctx context.Context, orgID, teamID string, cfg *AutoAssignLicenseConfig) error
}

// AutoRevokeRemovedLicenseSeatsResult details the outcome of an auto-revocation sweep.
type AutoRevokeRemovedLicenseSeatsResult struct {
	Status  string   `json:"status"` // "disabled", "members_unavailable", "ok"
	Pending []string `json:"pending"`
	Revoked []string `json:"revoked"`
	Failed  []string `json:"failed"`
}

// AutoRevokeRemovedLicenseSeatsUseCase manages the multi-day grace period
// countdown before reclaiming license seats from users removed from Git hosting providers.
type AutoRevokeRemovedLicenseSeatsUseCase struct {
	paramStore   IAutoLicenseParametersStore
	pruneUseCase *PruneRemovedLicenseSeatsUseCase
	logger       *slog.Logger
	clock        func() time.Time
}

// NewAutoRevokeRemovedLicenseSeatsUseCase instantiates the grace period manager.
func NewAutoRevokeRemovedLicenseSeatsUseCase(
	paramStore IAutoLicenseParametersStore,
	pruneUseCase *PruneRemovedLicenseSeatsUseCase,
	logger *slog.Logger,
) *AutoRevokeRemovedLicenseSeatsUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoRevokeRemovedLicenseSeatsUseCase{
		paramStore:   paramStore,
		pruneUseCase: pruneUseCase,
		logger:       logger,
		clock:        func() time.Time { return time.Now().UTC() },
	}
}

// Execute evaluates candidate seat revocations against the organization's grace period.
func (uc *AutoRevokeRemovedLicenseSeatsUseCase) Execute(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*AutoRevokeRemovedLicenseSeatsResult, error) {
	if uc.paramStore == nil || uc.pruneUseCase == nil {
		return &AutoRevokeRemovedLicenseSeatsResult{
			Status:  "disabled",
			Pending: make([]string, 0),
			Revoked: make([]string, 0),
			Failed:  make([]string, 0),
		}, nil
	}

	config, err := uc.paramStore.GetAutoAssignConfig(ctx, orgData.OrganizationID, orgData.TeamID)
	if err != nil || config == nil || !config.AutoRevokeRemovedUsers {
		return &AutoRevokeRemovedLicenseSeatsResult{
			Status:  "disabled",
			Pending: make([]string, 0),
			Revoked: make([]string, 0),
			Failed:  make([]string, 0),
		}, nil
	}

	// 1. Discover candidates missing from the Git provider in dryRun mode
	preview, err := uc.pruneUseCase.Execute(ctx, PruneRemovedLicenseSeatsParams{
		OrgData: orgData,
		DryRun:  true,
	})
	if err != nil || preview == nil || preview.Status != "ok" {
		uc.logger.WarnContext(ctx, "Skipping auto-revocation: provider members could not be retrieved",
			"orgID", orgData.OrganizationID,
			"teamID", orgData.TeamID,
		)
		return &AutoRevokeRemovedLicenseSeatsResult{
			Status:  "members_unavailable",
			Pending: make([]string, 0),
			Revoked: make([]string, 0),
			Failed:  make([]string, 0),
		}, nil
	}

	graceDays := config.RevokeGraceDays
	if graceDays <= 0 {
		graceDays = DefaultRevokeGraceDays
	}
	graceDuration := time.Duration(graceDays) * 24 * time.Hour

	now := uc.clock()
	previous := config.PendingRevocations
	if previous == nil {
		previous = make(map[string]string)
	}

	nextPending := make(map[string]string)
	var due []string

	for _, gitID := range preview.Candidates {
		firstSeenStr, exists := previous[gitID]
		var missingSince time.Time
		var parseErr error

		if exists {
			missingSince, parseErr = time.Parse(time.RFC3339, firstSeenStr)
		}
		if !exists || parseErr != nil {
			missingSince = now
		}

		nextPending[gitID] = missingSince.Format(time.RFC3339)

		if now.Sub(missingSince) >= graceDuration {
			due = append(due, gitID)
		}
	}

	revoked := make([]string, 0)
	failed := make([]string, 0)

	// 2. Revoke seats that matured past the grace period
	if len(due) > 0 {
		pruneResult, pruneErr := uc.pruneUseCase.Execute(ctx, PruneRemovedLicenseSeatsParams{
			OrgData: orgData,
			DryRun:  false,
			GitIDs:  due,
		})

		if pruneErr != nil {
			uc.logger.ErrorContext(ctx, "Failed executing seat revocation batch",
				"orgID", orgData.OrganizationID,
				"dueCount", len(due),
				"error", pruneErr,
			)
			failed = append(failed, due...)
		} else if pruneResult != nil {
			revoked = pruneResult.Revoked
			failed = pruneResult.Failed

			// Remove successfully revoked users from pending countdown
			for _, gitID := range revoked {
				delete(nextPending, gitID)
			}

			uc.logger.InfoContext(ctx, "Auto-revoked license seats for users removed from Git",
				"orgID", orgData.OrganizationID,
				"teamID", orgData.TeamID,
				"revokedCount", len(revoked),
				"failedCount", len(failed),
			)
		}
	}

	// 3. Persist updated countdown state
	config.PendingRevocations = nextPending
	if err := uc.paramStore.SaveAutoAssignConfig(ctx, orgData.OrganizationID, orgData.TeamID, config); err != nil {
		uc.logger.ErrorContext(ctx, "Failed saving pending revocations countdown timers",
			"orgID", orgData.OrganizationID,
			"teamID", orgData.TeamID,
			"error", err,
		)
	}

	pendingList := make([]string, 0, len(nextPending))
	for gitID := range nextPending {
		pendingList = append(pendingList, gitID)
	}
	sort.Strings(pendingList)

	return &AutoRevokeRemovedLicenseSeatsResult{
		Status:  "ok",
		Pending: pendingList,
		Revoked: revoked,
		Failed:  failed,
	}, nil
}
