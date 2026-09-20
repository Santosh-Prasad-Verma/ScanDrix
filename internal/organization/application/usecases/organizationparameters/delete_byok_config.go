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
	orgparams "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// DeleteBYOKConfigUseCase handles safe removal of customer-managed LLM configurations.
type DeleteBYOKConfigUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewDeleteBYOKConfigUseCase(repo orgparams.IOrganizationParametersRepository) *DeleteBYOKConfigUseCase {
	return &DeleteBYOKConfigUseCase{repo: repo}
}

func (uc *DeleteBYOKConfigUseCase) Execute(ctx context.Context, wsID uuid.UUID, modelID string) error {
	if wsID == uuid.Nil {
		return errors.New("workspace ID is required")
	}
	if uc.repo == nil {
		return nil
	}

	// Complete BYOK reset
	if modelID == "" {
		return uc.repo.Delete(ctx, wsID, orgparams.KeyBYOKConfig)
	}

	// Delete specific model ID and clean up orphans
	param, err := uc.repo.FindByKey(ctx, wsID, orgparams.KeyBYOKConfig)
	if err != nil || param == nil {
		return nil
	}

	var byokCfg orgparams.BYOKConfigValue
	if err := json.Unmarshal(param.ConfigValue, &byokCfg); err != nil {
		return nil
	}

	// Referential integrity: prevent deleting the currently active primary (main) model
	if byokCfg.Main != nil && (byokCfg.Main.ID == modelID || byokCfg.Main.ModelID == modelID) {
		return fmt.Errorf("model %q cannot be deleted because it is currently configured as the main review model", modelID)
	}

	// Referential integrity: check if referenced in model overrides
	overridesParam, _ := uc.repo.FindByKey(ctx, wsID, orgparams.KeyModelOverrides)
	if overridesParam != nil && len(overridesParam.ConfigValue) > 0 {
		var overrides []ModelOverride
		if err := json.Unmarshal(overridesParam.ConfigValue, &overrides); err == nil {
			for _, o := range overrides {
				if o.ModelID == modelID {
					scopeDesc := string(o.Scope)
					if o.RepositoryPath != "" {
						scopeDesc += " (" + o.RepositoryPath + ")"
					}
					return fmt.Errorf("model %q cannot be deleted because it is referenced in %s override", modelID, scopeDesc)
				}
			}
		}
	}

	var remainingModels []orgparams.BYOKModel
	usedCredIDs := make(map[string]bool)
	for _, m := range byokCfg.Models {
		if m.ID != modelID && m.ModelID != modelID {
			remainingModels = append(remainingModels, m)
			usedCredIDs[m.CredentialID] = true
		}
	}

	// If no models remain, delete config entirely
	if len(remainingModels) == 0 {
		return uc.repo.Delete(ctx, wsID, orgparams.KeyBYOKConfig)
	}

	// Prune orphaned credentials
	var remainingCreds []orgparams.BYOKCredential
	for _, c := range byokCfg.Credentials {
		if usedCredIDs[c.ID] {
			remainingCreds = append(remainingCreds, c)
		}
	}

	byokCfg.Models = remainingModels
	byokCfg.Credentials = remainingCreds
	newBytes, _ := json.Marshal(byokCfg)
	param.ConfigValue = newBytes

	_, err = uc.repo.Update(ctx, orgparams.OrganizationParametersFilter{
		WorkspaceID: &wsID,
		ConfigKey:   &param.ConfigKey,
	}, param)
	return err
}
