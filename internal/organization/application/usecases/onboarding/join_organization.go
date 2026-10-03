// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package onboardingusecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// UserAccount defines the minimal user identity fields needed for onboarding.
type UserAccount struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
}

// IUserAccountRepository provides user access for organization onboarding.
type IUserAccountRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*UserAccount, error)
	FindByWorkspaceID(ctx context.Context, wsID uuid.UUID) ([]*UserAccount, error)
	UpdateWorkspaceAndRole(ctx context.Context, userID, newWsID uuid.UUID, role, status string) (*UserAccount, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error
}

// JoinOrganizationInput defines input parameters for joining an organization.
type JoinOrganizationInput struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`

	// InvitationCode is the code from the invite link, when the join was
	// initiated from one. It is recorded for audit but is NOT validated:
	// there is no invitation repository in this codebase to validate it
	// against, and inventing one would silently break every join that does
	// not use an invite link.
	//
	// Until a real invitation store exists, callers MUST NOT treat the
	// presence of this field as proof that the join was authorised. The
	// transport-level check that the caller is authenticated and is acting
	// on their own account is what currently protects this path.
	// See AUDIT_REMEDIATION.md F-02.
	InvitationCode string `json:"invitation_code,omitempty"`
}

// JoinOrganizationUseCase manages onboarding users into a target organization and cleans up orphaned personal workspaces.
type JoinOrganizationUseCase struct {
	userRepo   IUserAccountRepository
	orgRepo    orgdomain.IOrganizationRepository
	teamRepo   teamdomain.ITeamRepository
	memberRepo memberdomain.ITeamMembersRepository
	paramRepo  paramdomain.IParametersRepository
}

// NewJoinOrganizationUseCase instantiates a new JoinOrganizationUseCase.
func NewJoinOrganizationUseCase(
	userRepo IUserAccountRepository,
	orgRepo orgdomain.IOrganizationRepository,
	teamRepo teamdomain.ITeamRepository,
	memberRepo memberdomain.ITeamMembersRepository,
	paramRepo paramdomain.IParametersRepository,
) *JoinOrganizationUseCase {
	return &JoinOrganizationUseCase{
		userRepo:   userRepo,
		orgRepo:    orgRepo,
		teamRepo:   teamRepo,
		memberRepo: memberRepo,
		paramRepo:  paramRepo,
	}
}

// Execute moves a user into the target organization workspace, provisions team membership, and executes workspace cleanup.
func (uc *JoinOrganizationUseCase) Execute(ctx context.Context, input JoinOrganizationInput) (*UserAccount, error) {
	if input.UserID == uuid.Nil || input.OrganizationID == uuid.Nil {
		return nil, errors.New("user ID and organization ID are required")
	}

	if uc.userRepo == nil || uc.orgRepo == nil || uc.teamRepo == nil || uc.memberRepo == nil {
		return nil, errors.New("required onboarding dependencies are uninitialized")
	}

	user, err := uc.userRepo.FindByID(ctx, input.UserID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	originalWsID := user.WorkspaceID
	if originalWsID == input.OrganizationID {
		return user, nil // Already in the target organization
	}

	// The invitation code is not validated (see JoinOrganizationInput); record
	// it so an audit trail exists for joins that presented one, without
	// logging the raw code.
	slog.Info("organization.join",
		"event", "organization.join",
		"user_id", input.UserID,
		"from_workspace_id", originalWsID,
		"to_organization_id", input.OrganizationID,
		"invitation_code_present", input.InvitationCode != "",
	)

	targetOrg, err := uc.orgRepo.FindByID(ctx, input.OrganizationID)
	if err != nil || targetOrg == nil {
		return nil, fmt.Errorf("target organization not found: %w", err)
	}

	// Locate primary team in target organization
	teams, err := uc.teamRepo.FindByWorkspaceID(ctx, input.OrganizationID)
	if err != nil || len(teams) == 0 {
		return nil, errors.New("no default team found in target organization")
	}
	targetTeam := teams[0]

	// Find or create team member
	existingMember, _ := uc.memberRepo.FindOne(ctx, memberdomain.TeamMemberFilter{
		UserID: &user.ID,
		TeamID: &targetTeam.UUID,
	})

	if existingMember == nil {
		newMember := memberdomain.NewTeamMemberEntity(
			input.OrganizationID,
			targetTeam.UUID,
			user.ID,
			user.DisplayName,
			user.Email,
			memberdomain.RoleMember,
		)
		if _, err := uc.memberRepo.Create(ctx, newMember); err != nil {
			return nil, fmt.Errorf("failed to create team member record: %w", err)
		}
	} else {
		updateData := *existingMember
		updateData.WorkspaceID = input.OrganizationID
		updateData.TeamID = targetTeam.UUID
		updateData.Role = memberdomain.RoleMember
		updateData.Status = true
		_, _ = uc.memberRepo.Update(ctx, memberdomain.TeamMemberFilter{UUID: &existingMember.UUID}, &updateData)
	}

	// Update user's workspace association
	targetStatus := "ACTIVE"
	if os.Getenv("API_CLOUD_MODE") == "true" || os.Getenv("SCANDRIX_CLOUD_MODE") == "true" {
		targetStatus = "PENDING_EMAIL"
	}
	updatedUser, err := uc.userRepo.UpdateWorkspaceAndRole(ctx, user.ID, input.OrganizationID, "CONTRIBUTOR", targetStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to update user workspace: %w", err)
	}

	// Clean up previous personal workspace if left empty
	if originalWsID != uuid.Nil && originalWsID != input.OrganizationID {
		uc.cleanUpOrphanedWorkspace(ctx, originalWsID)
	}

	return updatedUser, nil
}

// cleanUpOrphanedWorkspace removes empty teams, parameters, and workspace records for an orphaned personal tenant.
//
// It FAILS CLOSED. Every repository error aborts the cleanup rather than
// continuing: the original condition was `err == nil && len(remainingUsers) > 0`,
// which meant a transient database error skipped the early return and fell
// straight through to the deletes below. A momentary DB blip could therefore
// destroy a workspace that still had members in it.
func (uc *JoinOrganizationUseCase) cleanUpOrphanedWorkspace(ctx context.Context, wsID uuid.UUID) {
	remainingUsers, err := uc.userRepo.FindByWorkspaceID(ctx, wsID)
	if err != nil {
		slog.Warn("Skipping orphaned-workspace cleanup: user lookup failed; refusing to delete on unknown state",
			"workspace_id", wsID, "error", err)
		return
	}
	if len(remainingUsers) > 0 {
		slog.Info("Workspace retains active users; skipping tenant deletion", "workspace_id", wsID)
		return
	}

	teams, err := uc.teamRepo.FindByWorkspaceID(ctx, wsID)
	if err != nil {
		slog.Warn("Skipping orphaned-workspace cleanup: team lookup failed",
			"workspace_id", wsID, "error", err)
		return
	}
	for _, t := range teams {
		members, mErr := uc.memberRepo.Find(ctx, memberdomain.TeamMemberFilter{TeamID: &t.UUID})
		if mErr != nil {
			slog.Warn("Skipping team cleanup: member lookup failed", "team_id", t.UUID, "error", mErr)
			continue
		}
		if len(members) == 0 {
			if uc.paramRepo != nil {
				_ = uc.paramRepo.DeleteByTeamID(ctx, t.UUID)
			}
			_ = uc.teamRepo.Delete(ctx, t.UUID)
		}
	}

	// Verify remaining teams before removing the tenant row. Fails closed: an
	// error here must never be read as "no teams remain, safe to delete".
	remainingTeams, err := uc.teamRepo.FindByWorkspaceID(ctx, wsID)
	if err != nil {
		slog.Warn("Skipping organization deletion: team re-verification failed",
			"workspace_id", wsID, "error", err)
		return
	}
	if len(remainingTeams) == 0 {
		if delErr := uc.orgRepo.Delete(ctx, wsID); delErr != nil {
			slog.Error("Failed to delete orphaned organization", "workspace_id", wsID, "error", delErr)
			return
		}
		slog.Info("Orphaned personal workspace purged successfully", "workspace_id", wsID)
	}
}
