// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgusecases

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/organization"
)

// GetOrganizationNameUseCase resolves the display name of an organization.
type GetOrganizationNameUseCase struct {
	repo orgdomain.IOrganizationRepository
}

func NewGetOrganizationNameUseCase(repo orgdomain.IOrganizationRepository) *GetOrganizationNameUseCase {
	return &GetOrganizationNameUseCase{repo: repo}
}

func (uc *GetOrganizationNameUseCase) Execute(ctx context.Context, orgID uuid.UUID) (string, error) {
	if orgID == uuid.Nil {
		return "", errors.New("organization ID is required")
	}
	if uc.repo == nil {
		return "Workspace", nil
	}
	org, err := uc.repo.FindByID(ctx, orgID)
	if err != nil || org == nil {
		return "Workspace", err
	}
	return org.Name, nil
}
