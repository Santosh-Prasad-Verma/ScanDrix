// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package memberusecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	onboardingusecases "github.com/scandrix/backend/internal/organization/application/usecases/onboarding"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// TeamReader resolves team names for member removal reporting.
type TeamReader interface {
	FindByID(ctx context.Context, wsID, id uuid.UUID) (*teamdomain.TeamEntity, error)
}

// DeleteTeamMemberUseCase removes a user from a team.
type DeleteTeamMemberUseCase struct {
	memberRepo memberdomain.ITeamMembersRepository
	userRepo   onboardingusecases.IUserAccountRepository
	teamRepo   TeamReader
}

// NewDeleteTeamMemberUseCase creates a DeleteTeamMemberUseCase.
func NewDeleteTeamMemberUseCase(memberRepo memberdomain.ITeamMembersRepository) *DeleteTeamMemberUseCase {
	return &DeleteTeamMemberUseCase{memberRepo: memberRepo}
}

// WithUserRepo injects user repository to support cascading account deletion when leaving last team.
func (uc *DeleteTeamMemberUseCase) WithUserRepo(userRepo onboardingusecases.IUserAccountRepository) *DeleteTeamMemberUseCase {
	uc.userRepo = userRepo
	return uc
}

// WithTeamRepo injects team repository to resolve real team names when a user belongs to multiple teams.
func (uc *DeleteTeamMemberUseCase) WithTeamRepo(teamRepo TeamReader) *DeleteTeamMemberUseCase {
	uc.teamRepo = teamRepo
	return uc
}

// Execute removes a team member by UUID, preventing self-removal and cascading user deletion when appropriate.
// Returns list of other teams the user belongs to if not completely removed.
func (uc *DeleteTeamMemberUseCase) Execute(ctx context.Context, wsID, memberUUID uuid.UUID, actorUserID *uuid.UUID, removeAll bool) ([]string, error) {
	if memberUUID == uuid.Nil {
		return nil, errors.New("member UUID is required")
	}
	if uc.memberRepo == nil {
		return nil, errors.New("team members repository unavailable")
	}

	memberToRemove, err := uc.memberRepo.FindByID(ctx, wsID, memberUUID)
	if err != nil || memberToRemove == nil {
		return nil, errors.New("team member not found")
	}

	// Prevent user from removing their own account
	if actorUserID != nil && memberToRemove.UserID == *actorUserID {
		return nil, errors.New("you cannot remove your own account")
	}

	// Find all team memberships for this user
	activeStatus := true
	relatedMembers, err := uc.memberRepo.Find(ctx, memberdomain.TeamMemberFilter{
		WorkspaceID: &wsID,
		UserID:      &memberToRemove.UserID,
		Status:      &activeStatus,
	})
	if err != nil {
		relatedMembers = []*memberdomain.TeamMemberEntity{memberToRemove}
	}

	membersToDelete := []*memberdomain.TeamMemberEntity{memberToRemove}
	if removeAll {
		membersToDelete = relatedMembers
	}

	deleteUUIDs := make([]uuid.UUID, 0, len(membersToDelete))
	for _, m := range membersToDelete {
		deleteUUIDs = append(deleteUUIDs, m.UUID)
		_ = uc.memberRepo.Delete(ctx, wsID, m.TeamID, m.UserID)
	}
	_ = uc.memberRepo.DeleteMembers(ctx, wsID, deleteUUIDs)

	count, _ := uc.memberRepo.CountByUser(ctx, wsID, memberToRemove.UserID, &activeStatus)

	if count <= 0 || removeAll {
		if uc.userRepo != nil {
			_ = uc.userRepo.DeleteUser(ctx, memberToRemove.UserID)
		}
		return nil, nil
	}

	// User is still in other teams; return list of other team IDs / identifiers
	var otherTeams []string
	for _, m := range relatedMembers {
		if m.TeamID != memberToRemove.TeamID {
			teamName := fmt.Sprintf("team_%s", m.TeamID.String()[:8])
			if uc.teamRepo != nil {
				if t, err := uc.teamRepo.FindByID(ctx, wsID, m.TeamID); err == nil && t != nil && t.Name != "" {
					teamName = t.Name
				}
			}
			otherTeams = append(otherTeams, teamName)
		}
	}

	return otherTeams, nil
}

// ExecuteByID removes a user from a team using teamID and userID within a workspace.
func (uc *DeleteTeamMemberUseCase) ExecuteByID(ctx context.Context, wsID, teamID, userID uuid.UUID) error {
	if wsID == uuid.Nil || teamID == uuid.Nil || userID == uuid.Nil {
		return errors.New("workspace ID, team ID and user ID are required")
	}
	if uc.memberRepo == nil {
		return errors.New("team members repository unavailable")
	}
	return uc.memberRepo.Delete(ctx, wsID, teamID, userID)
}
