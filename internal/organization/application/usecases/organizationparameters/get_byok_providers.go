// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm/providers"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	orgparams "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// ProviderInfo represents supported AI model provider metadata.
type ProviderInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Configured  bool   `json:"configured"`
	RequiresURL bool   `json:"requiresUrl"`
}

// GetBYOKProvidersUseCase lists all supported AI providers and their configuration status dynamically.
type GetBYOKProvidersUseCase struct {
	repo orgparams.IOrganizationParametersRepository
}

func NewGetBYOKProvidersUseCase(repo orgparams.IOrganizationParametersRepository) *GetBYOKProvidersUseCase {
	return &GetBYOKProvidersUseCase{repo: repo}
}

func (uc *GetBYOKProvidersUseCase) Execute(ctx context.Context, wsID uuid.UUID) ([]ProviderInfo, error) {
	// 1. Gather all providers dynamically registered in ScanDrix kernel
	modules := kernel.List()
	supported := make([]ProviderInfo, 0, len(modules))

	for _, mod := range modules {
		id := mod.ID()
		desc := providers.DescribeProviderID(mod, id)
		supported = append(supported, ProviderInfo{
			ID:          id,
			Name:        mod.Label(),
			Configured:  false,
			RequiresURL: desc.RequiresBaseURL,
		})
	}

	// Fallback to standard list if registry was empty in a restricted test environment
	if len(supported) == 0 {
		supported = []ProviderInfo{
			{ID: "openai", Name: "OpenAI", Configured: false, RequiresURL: false},
			{ID: "anthropic", Name: "Anthropic", Configured: false, RequiresURL: false},
			{ID: "gemini", Name: "Google Gemini", Configured: false, RequiresURL: false},
			{ID: "vertex", Name: "Google Vertex AI", Configured: false, RequiresURL: false},
			{ID: "bedrock", Name: "AWS Bedrock", Configured: false, RequiresURL: false},
			{ID: "azure", Name: "Azure OpenAI", Configured: false, RequiresURL: true},
			{ID: "openrouter", Name: "OpenRouter", Configured: false, RequiresURL: false},
			{ID: "openai_compatible", Name: "OpenAI Compatible", Configured: false, RequiresURL: true},
		}
	}

	if wsID == uuid.Nil || uc.repo == nil {
		return supported, nil
	}

	// 2. Query workspace parameters for real configured BYOK credentials
	param, err := uc.repo.FindByKey(ctx, wsID, orgparams.KeyBYOKConfig)
	if err != nil || param == nil || len(param.ConfigValue) == 0 {
		return supported, nil
	}

	var byokCfg orgparams.BYOKConfigValue
	if err := json.Unmarshal(param.ConfigValue, &byokCfg); err != nil {
		return supported, nil
	}

	configuredMap := make(map[string]bool)
	for _, cred := range byokCfg.Credentials {
		if strings.TrimSpace(cred.APIKey) != "" || cred.Provider == "bedrock" || cred.Provider == "vertex" {
			configuredMap[strings.ToLower(strings.TrimSpace(cred.Provider))] = true
		}
	}

	for i := range supported {
		pID := strings.ToLower(supported[i].ID)
		if configuredMap[pID] {
			supported[i].Configured = true
		}
	}

	return supported, nil
}
