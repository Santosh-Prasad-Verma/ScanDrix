// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// ListModelOverridesUseCase lists all active model overrides for a workspace.
type ListModelOverridesUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewListModelOverridesUseCase(repo orgparams.IOrganizationParametersRepository) *ListModelOverridesUseCase {
	return &ListModelOverridesUseCase{repo: repo}
}

func (uc *ListModelOverridesUseCase) Execute(ctx context.Context, wsID uuid.UUID) ([]ModelOverride, error) {
	if wsID == uuid.Nil || uc.repo == nil {
		return []ModelOverride{}, nil
	}

	param, err := uc.repo.FindByKey(ctx, wsID, orgparams.KeyModelOverrides)
	if err != nil || param == nil {
		return []ModelOverride{}, nil
	}

	var overrides []ModelOverride
	if err := json.Unmarshal(param.ConfigValue, &overrides); err != nil {
		return []ModelOverride{}, nil
	}
	return overrides, nil
}
