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

// UserProfileUpdater defines interface to update user profile information such as phone.
type UserProfileUpdater interface {
	UpdatePhone(ctx context.Context, userID uuid.UUID, phone string) error
}

// UpdateInfosUseCase updates organization metadata and user profile phone.
type UpdateInfosUseCase struct {
	orgRepo        orgdomain.IOrganizationRepository
	profileUpdater UserProfileUpdater
}

func NewUpdateInfosUseCase(orgRepo orgdomain.IOrganizationRepository) *UpdateInfosUseCase {
	return &UpdateInfosUseCase{orgRepo: orgRepo}
}

func (uc *UpdateInfosUseCase) WithProfileUpdater(updater UserProfileUpdater) *UpdateInfosUseCase {
	uc.profileUpdater = updater
	return uc
}

func (uc *UpdateInfosUseCase) Execute(ctx context.Context, orgID uuid.UUID, newName string) error {
	return uc.ExecuteWithPhone(ctx, orgID, nil, newName, nil)
}

func (uc *UpdateInfosUseCase) ExecuteWithPhone(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, newName string, phone *string) error {
	if orgID == uuid.Nil {
		return errors.New("organization ID is required")
	}
	if uc.orgRepo != nil && newName != "" {
		_, err := uc.orgRepo.Update(ctx, orgdomain.OrganizationFilter{
			UUID: &orgID,
		}, &orgdomain.OrganizationEntity{
			Name: newName,
		})
		if err != nil {
			return err
		}
	}

	if phone != nil && *phone != "" && userID != nil && *userID != uuid.Nil && uc.profileUpdater != nil {
		_ = uc.profileUpdater.UpdatePhone(ctx, *userID, *phone)
	}

	return nil
}
