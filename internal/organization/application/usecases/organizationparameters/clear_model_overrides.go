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

// ClearModelOverridesUseCase clears or scopes model overrides.
type ClearModelOverridesUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewClearModelOverridesUseCase(repo orgparams.IOrganizationParametersRepository) *ClearModelOverridesUseCase {
	return &ClearModelOverridesUseCase{repo: repo}
}

func (uc *ClearModelOverridesUseCase) Execute(ctx context.Context, wsID uuid.UUID, repositoryPath string) error {
	if wsID == uuid.Nil || uc.repo == nil {
		return nil
	}

	if repositoryPath == "" {
		return uc.repo.Delete(ctx, wsID, orgparams.KeyModelOverrides)
	}

	param, err := uc.repo.FindByKey(ctx, wsID, orgparams.KeyModelOverrides)
	if err != nil || param == nil {
		return nil
	}

	var overrides []ModelOverride
	if err := json.Unmarshal(param.ConfigValue, &overrides); err != nil {
		return nil
	}

	var retained []ModelOverride
	for _, o := range overrides {
		if o.RepositoryPath != repositoryPath {
			retained = append(retained, o)
		}
	}

	bytes, _ := json.Marshal(retained)
	param.ConfigValue = bytes
	_, err = uc.repo.Update(ctx, orgparams.OrganizationParametersFilter{
		WorkspaceID: &wsID,
		ConfigKey:   &param.ConfigKey,
	}, param)
	return err
}
