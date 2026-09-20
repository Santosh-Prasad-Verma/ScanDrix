// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package teamusecases

import (
	"context"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/team"
)

// ListTeamsUseCase retrieves all squads within a workspace.
type ListTeamsUseCase struct {
	teamRepo teamdomain.ITeamRepository
}

func NewListTeamsUseCase(teamRepo teamdomain.ITeamRepository) *ListTeamsUseCase {
	return &ListTeamsUseCase{teamRepo: teamRepo}
}

func (uc *ListTeamsUseCase) Execute(ctx context.Context, wsID uuid.UUID) ([]*teamdomain.TeamEntity, error) {
	return uc.ExecuteWithRole(ctx, wsID, nil, "")
}

// ExecuteWithRole applies RBAC: OWNERS and ADMINS see all teams; regular members/contributors see only teams they belong to.
func (uc *ListTeamsUseCase) ExecuteWithRole(ctx context.Context, wsID uuid.UUID, userID *uuid.UUID, role string) ([]*teamdomain.TeamEntity, error) {
	if wsID == uuid.Nil || uc.teamRepo == nil {
		return []*teamdomain.TeamEntity{}, nil
	}

	// Owners, Admins, or unspecified callers view all teams
	if userID == nil || *userID == uuid.Nil || role == "OWNER" || role == "ADMIN" || role == "" {
		return uc.teamRepo.Find(ctx, teamdomain.TeamFilter{
			WorkspaceID: &wsID,
		})
	}

	// Regular members/contributors only see teams they are assigned to
	return uc.teamRepo.GetTeamsByUserID(ctx, *userID, wsID)
}
