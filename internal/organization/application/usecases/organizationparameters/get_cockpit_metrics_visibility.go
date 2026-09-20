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

// GetCockpitMetricsVisibilityUseCase returns the metrics visibility configuration.
type GetCockpitMetricsVisibilityUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewGetCockpitMetricsVisibilityUseCase(repo orgparams.IOrganizationParametersRepository) *GetCockpitMetricsVisibilityUseCase {
	return &GetCockpitMetricsVisibilityUseCase{repo: repo}
}

func (uc *GetCockpitMetricsVisibilityUseCase) Execute(ctx context.Context, wsID uuid.UUID) (*orgparams.CockpitMetricsVisibilityConfigValue, error) {
	defaultVisibility := &orgparams.CockpitMetricsVisibilityConfigValue{
		OverallDORAMetrics:  true,
		DeploymentFrequency: true,
		LeadTimeForChanges:  true,
		ChangeFailureRate:   true,
		TimeToRestore:       true,
		PRVelocity:          true,
		PRSizeMetrics:       true,
		DrixyImpactMetrics:  true,
	}

	if wsID == uuid.Nil || uc.repo == nil {
		return defaultVisibility, nil
	}

	param, err := uc.repo.FindByKey(ctx, wsID, orgparams.KeyCockpitMetricsVisibility)
	if err != nil || param == nil {
		return defaultVisibility, nil
	}

	var custom orgparams.CockpitMetricsVisibilityConfigValue
	if err := json.Unmarshal(param.ConfigValue, &custom); err != nil {
		return defaultVisibility, nil
	}

	return &custom, nil
}
