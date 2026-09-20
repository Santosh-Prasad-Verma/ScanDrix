// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// CreateOrUpdateUseCase validates and persists organization parameter updates.
type CreateOrUpdateUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewCreateOrUpdateUseCase(repo orgparams.IOrganizationParametersRepository) *CreateOrUpdateUseCase {
	return &CreateOrUpdateUseCase{repo: repo}
}

func (uc *CreateOrUpdateUseCase) Execute(ctx context.Context, wsID uuid.UUID, key orgparams.ParameterKey, val any, desc string) (*orgparams.OrganizationParametersEntity, error) {
	if wsID == uuid.Nil {
		return nil, errors.New("workspace ID is required")
	}
	if uc.repo == nil {
		return nil, errors.New("repository unavailable")
	}

	var processedVal = val

	// Special handling for BYOK configuration
	if key == orgparams.KeyBYOKConfig {
		valBytes, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("invalid BYOK payload: %w", err)
		}

		var byokCfg orgparams.BYOKConfigValue
		if err := json.Unmarshal(valBytes, &byokCfg); err != nil {
			return nil, fmt.Errorf("malformed BYOK schema: %w", err)
		}

		// Fetch existing to preserve encrypted credentials if masked
		existing, _ := uc.repo.FindByKey(ctx, wsID, key)
		var existingCfg orgparams.BYOKConfigValue
		if existing != nil {
			_ = json.Unmarshal(existing.ConfigValue, &existingCfg)
		}

		credMap := make(map[string]orgparams.BYOKCredential)
		for _, c := range existingCfg.Credentials {
			credMap[c.ID] = c
		}

		for i, cred := range byokCfg.Credentials {
			if err := AssertSafeURL(cred.BaseURL); err != nil {
				return nil, fmt.Errorf("insecure endpoint for provider %s: %w", cred.Provider, err)
			}

			if IsMasked(cred.APIKey) {
				if oldCred, exists := credMap[cred.ID]; exists {
					byokCfg.Credentials[i].APIKey = oldCred.APIKey
				} else {
					return nil, fmt.Errorf("cannot preserve masked credential without existing reference: %s", cred.ID)
				}
			} else if cred.APIKey != "" {
				enc, err := EncryptSecret(cred.APIKey)
				if err != nil {
					return nil, fmt.Errorf("failed encrypting credential: %w", err)
				}
				byokCfg.Credentials[i].APIKey = enc
			}
		}

		processedVal = byokCfg
	}

	entity, err := orgparams.NewOrganizationParametersEntity(wsID, key, processedVal, desc)
	if err != nil {
		return nil, err
	}

	existing, _ := uc.repo.FindByKey(ctx, wsID, key)
	if existing == nil {
		return uc.repo.Create(ctx, entity)
	}

	return uc.repo.Update(ctx, orgparams.OrganizationParametersFilter{
		WorkspaceID: &wsID,
		ConfigKey:   &key,
	}, entity)
}
