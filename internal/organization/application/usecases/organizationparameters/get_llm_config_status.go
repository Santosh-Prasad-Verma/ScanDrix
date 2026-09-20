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

// LLMConfigStatus reports health and readiness of the AI review pipeline.
type LLMConfigStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	ModelID    string `json:"modelId"`
	Status     string `json:"status"` // ready, degraded, unconfigured
}

// GetLLMConfigStatusUseCase inspects current model configurations.
type GetLLMConfigStatusUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewGetLLMConfigStatusUseCase(repo orgparams.IOrganizationParametersRepository) *GetLLMConfigStatusUseCase {
	return &GetLLMConfigStatusUseCase{repo: repo}
}

func (uc *GetLLMConfigStatusUseCase) Execute(ctx context.Context, wsID uuid.UUID) (*LLMConfigStatus, error) {
	status := &LLMConfigStatus{
		Configured: true,
		Provider:   "managed",
		ModelID:    "claude-3-5-sonnet",
		Status:     "ready",
	}

	if wsID == uuid.Nil || uc.repo == nil {
		return status, nil
	}

	param, err := uc.repo.FindByKey(ctx, wsID, orgparams.KeyBYOKConfig)
	if err == nil && param != nil {
		var byokCfg orgparams.BYOKConfigValue
		if err := json.Unmarshal(param.ConfigValue, &byokCfg); err == nil && len(byokCfg.Models) > 0 {
			status.Provider = byokCfg.Models[0].Provider
			status.ModelID = byokCfg.Models[0].ModelID
		}
	}

	return status, nil
}
