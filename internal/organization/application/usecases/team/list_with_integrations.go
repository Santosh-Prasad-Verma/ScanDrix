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

// ListTeamsWithIntegrationsUseCase enriches teams with integration statuses.
type ListTeamsWithIntegrationsUseCase struct {
	teamRepo teamdomain.ITeamRepository
}

func NewListTeamsWithIntegrationsUseCase(teamRepo teamdomain.ITeamRepository) *ListTeamsWithIntegrationsUseCase {
	return &ListTeamsWithIntegrationsUseCase{teamRepo: teamRepo}
}

func (uc *ListTeamsWithIntegrationsUseCase) Execute(ctx context.Context, wsID uuid.UUID) ([]*teamdomain.TeamWithIntegrations, error) {
	if wsID == uuid.Nil || uc.teamRepo == nil {
		return []*teamdomain.TeamWithIntegrations{}, nil
	}
	return uc.teamRepo.ListWithIntegrations(ctx, wsID)
}
