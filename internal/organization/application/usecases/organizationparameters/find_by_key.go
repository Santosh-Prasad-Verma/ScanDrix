// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// FindByKeyUseCase retrieves an organization parameter with secret masking for API safety.
type FindByKeyUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewFindByKeyUseCase(repo orgparams.IOrganizationParametersRepository) *FindByKeyUseCase {
	return &FindByKeyUseCase{repo: repo}
}

func (uc *FindByKeyUseCase) Execute(ctx context.Context, wsID uuid.UUID, key orgparams.ParameterKey, maskSecrets bool) (*orgparams.OrganizationParametersEntity, error) {
	if wsID == uuid.Nil {
		return nil, errors.New("workspace ID is required")
	}
	if uc.repo == nil {
		return nil, errors.New("repository unavailable")
	}

	entity, err := uc.repo.FindByKey(ctx, wsID, key)
	if err != nil || entity == nil {
		return nil, err
	}

	if key == orgparams.KeyBYOKConfig && maskSecrets {
		var byokCfg orgparams.BYOKConfigValue
		if err := json.Unmarshal(entity.ConfigValue, &byokCfg); err == nil {
			for i := range byokCfg.Credentials {
				byokCfg.Credentials[i].APIKey = MaskSecret(byokCfg.Credentials[i].APIKey)
			}
			maskedBytes, _ := json.Marshal(byokCfg)
			// Return a safe copy without mutating storage
			safeCopy := *entity
			safeCopy.ConfigValue = maskedBytes
			return &safeCopy, nil
		}
	}

	return entity, nil
}
