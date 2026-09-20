package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

const (
	ScanDrixIssuesIntegrationID = "scandrix_issues_integration"
)

// IAuditLogEmitter emits immutable audit trail records for compliance.
type IAuditLogEmitter interface {
	EmitIntegrationEvent(ctx context.Context, orgID, teamID, userID, userEmail, action, platform string) error
}

// IAuthIntegrationStore manages secure credentials and OAuth tokens.
type IAuthIntegrationStore interface {
	SaveAuthIntegration(ctx context.Context, orgID, teamID string, provider models.SCMProvider, authMode string, credentials map[string]any) error
	DeleteAuthIntegration(ctx context.Context, orgID, teamID string) error
}

// ILicenseSeatService manages enterprise seats and user allocations.
type ILicenseSeatService interface {
	GetLicensedUsers(ctx context.Context, orgID, teamID string) ([]string, error)
	RevokeSeat(ctx context.Context, orgID, teamID, gitID string) error
}

// IAutoLicenseConfigStore reads and updates auto license assignment rules.
type IAutoLicenseConfigStore interface {
	GetAutoAssignAllowedUsers(ctx context.Context, orgID, teamID string) ([]string, error)
	SaveAutoAssignAllowedUsers(ctx context.Context, orgID, teamID string, allowedUsers []string) error
}

// IDrixyRulesCascadeService deactivates rules when integrations are removed.
type IDrixyRulesCascadeService interface {
	InactivateRulesForRepositories(ctx context.Context, orgID string, repoIDs []string) error
	DeletePullRequestMessages(ctx context.Context, orgID string, repoIDs []string) error
}

// ═══════════════════════════════════════════════════════════════
// 1. CreateIntegrationUseCase (translates create-integration.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type CreateIntegrationParams struct {
	OrganizationID  string
	TeamID          string
	UserID          string
	UserEmail       string
	Provider        models.SCMProvider
	AuthMode        string
	Credentials     map[string]any
}

type CreateIntegrationUseCase struct {
	authStore  IAuthIntegrationStore
	auditLog   IAuditLogEmitter
}

func NewCreateIntegrationUseCase(
	authStore IAuthIntegrationStore,
	auditLog IAuditLogEmitter,
) *CreateIntegrationUseCase {
	return &CreateIntegrationUseCase{
		authStore: authStore,
		auditLog:  auditLog,
	}
}

func (uc *CreateIntegrationUseCase) Execute(ctx context.Context, params CreateIntegrationParams) error {
	if strings.TrimSpace(params.OrganizationID) == "" || strings.TrimSpace(params.TeamID) == "" {
		return fmt.Errorf("organizationId and teamId are required")
	}

	authMode := params.AuthMode
	if authMode == "" {
		authMode = "oauth"
	}

	if uc.authStore != nil {
		if err := uc.authStore.SaveAuthIntegration(ctx, params.OrganizationID, params.TeamID, params.Provider, authMode, params.Credentials); err != nil {
			return fmt.Errorf("failed to save auth integration: %w", err)
		}
	}

	if uc.auditLog != nil {
		_ = uc.auditLog.EmitIntegrationEvent(ctx, params.OrganizationID, params.TeamID, params.UserID, params.UserEmail, "CREATE", string(params.Provider))
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════
// 2. DeleteIntegrationUseCase (translates delete-integration.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type DeleteIntegrationUseCase struct {
	codeManagement contracts.ICodeManagementService
	authStore      IAuthIntegrationStore
	auditLog       IAuditLogEmitter
}

func NewDeleteIntegrationUseCase(
	codeManagement contracts.ICodeManagementService,
	authStore IAuthIntegrationStore,
	auditLog IAuditLogEmitter,
) *DeleteIntegrationUseCase {
	return &DeleteIntegrationUseCase{
		codeManagement: codeManagement,
		authStore:      authStore,
		auditLog:       auditLog,
	}
}

func (uc *DeleteIntegrationUseCase) Execute(ctx context.Context, orgID, teamID, userID, userEmail string) error {
	orgData := types.OrganizationAndTeamData{
		OrganizationID: orgID,
		TeamID:         teamID,
	}

	// 1. Delete webhooks from remote Git provider (best-effort)
	if uc.codeManagement != nil {
		if err := uc.codeManagement.DeleteWebhook(ctx, orgData); err != nil {
			slog.WarnContext(ctx, "Error deleting webhooks from remote provider — proceeding with local cleanup",
				"organizationId", orgID,
				"teamId", teamID,
				"error", err,
			)
		}
	}

	// 2. Delete auth integration from database
	if uc.authStore != nil {
		if err := uc.authStore.DeleteAuthIntegration(ctx, orgID, teamID); err != nil {
			return fmt.Errorf("failed to delete auth integration: %w", err)
		}
	}

	// 3. Emit audit log
	if uc.auditLog != nil {
		_ = uc.auditLog.EmitIntegrationEvent(ctx, orgID, teamID, userID, userEmail, "DELETE", "UNKNOWN")
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════
// 3. DeleteIntegrationAndRepositoriesUseCase (translates delete-integration-and-repositories.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type DeleteIntegrationAndRepositoriesUseCase struct {
	deleteIntegration *DeleteIntegrationUseCase
	cascadeService    IDrixyRulesCascadeService
	configStore       IRepositoryConfigStore
}

func NewDeleteIntegrationAndRepositoriesUseCase(
	deleteIntegration *DeleteIntegrationUseCase,
	cascadeService IDrixyRulesCascadeService,
	configStore IRepositoryConfigStore,
) *DeleteIntegrationAndRepositoriesUseCase {
	return &DeleteIntegrationAndRepositoriesUseCase{
		deleteIntegration: deleteIntegration,
		cascadeService:    cascadeService,
		configStore:       configStore,
	}
}

func (uc *DeleteIntegrationAndRepositoriesUseCase) Execute(ctx context.Context, orgID, teamID, userID, userEmail string) error {
	// 1. Get repository IDs before deletion
	var repoIDs []string
	if uc.configStore != nil {
		if repos, err := uc.configStore.GetRepositories(ctx, orgID, teamID); err == nil {
			for _, r := range repos {
				if r != nil {
					repoIDs = append(repoIDs, fmt.Sprintf("%v", r.ID))
				}
			}
		}
	}

	// 2. Delete integration
	if uc.deleteIntegration != nil {
		_ = uc.deleteIntegration.Execute(ctx, orgID, teamID, userID, userEmail)
	}

	// 3. Clear repository configuration
	if uc.configStore != nil {
		_ = uc.configStore.SaveRepositories(ctx, orgID, teamID, []*types.Repositories{})
	}

	// 4. Cascade cleanup: delete PR messages & inactivate Drixy Rules
	if uc.cascadeService != nil && len(repoIDs) > 0 {
		_ = uc.cascadeService.DeletePullRequestMessages(ctx, orgID, repoIDs)
		_ = uc.cascadeService.InactivateRulesForRepositories(ctx, orgID, repoIDs)
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════
// 4. UpdateAutoLicenseAllowedUsersUseCase (translates update-auto-license-allowed-users.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type UpdateAutoLicenseAllowedUsersUseCase struct {
	configStore    IAutoLicenseConfigStore
	codeManagement contracts.ICodeManagementService
}

func NewUpdateAutoLicenseAllowedUsersUseCase(
	configStore IAutoLicenseConfigStore,
	codeManagement contracts.ICodeManagementService,
) *UpdateAutoLicenseAllowedUsersUseCase {
	return &UpdateAutoLicenseAllowedUsersUseCase{
		configStore:    configStore,
		codeManagement: codeManagement,
	}
}

func (uc *UpdateAutoLicenseAllowedUsersUseCase) Execute(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	includeCurrentUser bool,
) error {
	if strings.TrimSpace(orgData.OrganizationID) == "" {
		return fmt.Errorf("organizationId is required")
	}

	if uc.configStore == nil {
		return nil
	}

	allowed, err := uc.configStore.GetAutoAssignAllowedUsers(ctx, orgData.OrganizationID, orgData.TeamID)
	if err != nil {
		return err
	}

	set := make(map[string]bool)
	for _, id := range allowed {
		set[id] = true
	}

	if includeCurrentUser && uc.codeManagement != nil {
		user, err := uc.codeManagement.GetCurrentUser(ctx, orgData)
		if err == nil && user != nil {
			curID := user.ID
			if curID == "" {
				curID = user.Username
			}
			if curID != "" {
				set[curID] = true
			}
		}
	}

	merged := make([]string, 0, len(set))
	for id := range set {
		merged = append(merged, id)
	}

	return uc.configStore.SaveAutoAssignAllowedUsers(ctx, orgData.OrganizationID, orgData.TeamID, merged)
}

// ═══════════════════════════════════════════════════════════════
// 5. PruneRemovedLicenseSeatsUseCase (translates prune-removed-license-seats.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type PruneRemovedLicenseSeatsResult struct {
	Status     string   `json:"status"` // "ok" or "members_unavailable"
	Candidates []string `json:"candidates"`
	Revoked    []string `json:"revoked"`
	Failed     []string `json:"failed"`
}

type PruneRemovedLicenseSeatsParams struct {
	OrgData types.OrganizationAndTeamData
	DryRun  bool
	GitIDs  []string
}

type PruneRemovedLicenseSeatsUseCase struct {
	memberListService *services.OrganizationMemberListService
	licenseService    ILicenseSeatService
}

func NewPruneRemovedLicenseSeatsUseCase(
	memberListService *services.OrganizationMemberListService,
	licenseService ILicenseSeatService,
) *PruneRemovedLicenseSeatsUseCase {
	return &PruneRemovedLicenseSeatsUseCase{
		memberListService: memberListService,
		licenseService:    licenseService,
	}
}

func (uc *PruneRemovedLicenseSeatsUseCase) Execute(
	ctx context.Context,
	params PruneRemovedLicenseSeatsParams,
) (*PruneRemovedLicenseSeatsResult, error) {
	if uc.memberListService == nil {
		return &PruneRemovedLicenseSeatsResult{
			Status: "members_unavailable",
		}, nil
	}

	memberResult, err := uc.memberListService.Fetch(ctx, params.OrgData, false)
	if err != nil || memberResult == nil || memberResult.Status != "ok" {
		slog.WarnContext(ctx, "Skipping license seat prune: organization members could not be confirmed",
			"organizationId", params.OrgData.OrganizationID,
			"teamId", params.OrgData.TeamID,
		)
		return &PruneRemovedLicenseSeatsResult{
			Status: "members_unavailable",
		}, nil
	}

	confirmedMembers := make(map[string]bool)
	for _, m := range memberResult.Members {
		confirmedMembers[m.ID] = true
	}

	if uc.licenseService == nil {
		return &PruneRemovedLicenseSeatsResult{
			Status: "ok",
		}, nil
	}

	activeSeats, err := uc.licenseService.GetLicensedUsers(ctx, params.OrgData.OrganizationID, params.OrgData.TeamID)
	if err != nil {
		return nil, err
	}

	requestedFilter := make(map[string]bool)
	hasRequested := len(params.GitIDs) > 0
	for _, gid := range params.GitIDs {
		requestedFilter[gid] = true
	}

	var candidates []string
	for _, seat := range activeSeats {
		if confirmedMembers[seat] {
			continue
		}
		if hasRequested && !requestedFilter[seat] {
			continue
		}
		candidates = append(candidates, seat)
	}

	var revoked []string
	var failed []string

	if !params.DryRun {
		for _, c := range candidates {
			if err := uc.licenseService.RevokeSeat(ctx, params.OrgData.OrganizationID, params.OrgData.TeamID, c); err != nil {
				failed = append(failed, c)
			} else {
				revoked = append(revoked, c)
			}
		}
	}

	return &PruneRemovedLicenseSeatsResult{
		Status:     "ok",
		Candidates: candidates,
		Revoked:    revoked,
		Failed:     failed,
	}, nil
}
