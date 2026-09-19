// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgusecases

import (
	"context"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/organization"
)

// GetReleaseTrackUseCase returns the release track configured for an organization.
type GetReleaseTrackUseCase struct {
	repo orgdomain.IOrganizationRepository
}

func NewGetReleaseTrackUseCase(repo orgdomain.IOrganizationRepository) *GetReleaseTrackUseCase {
	return &GetReleaseTrackUseCase{repo: repo}
}

func (uc *GetReleaseTrackUseCase) Execute(ctx context.Context, orgID uuid.UUID) (orgdomain.ReleaseTrack, error) {
	if orgID == uuid.Nil || uc.repo == nil {
		return orgdomain.DefaultReleaseTrack, nil
	}
	org, err := uc.repo.FindByID(ctx, orgID)
	if err != nil || org == nil || org.ReleaseTrack == "" {
		return orgdomain.DefaultReleaseTrack, nil
	}
	return org.ReleaseTrack, nil
}
