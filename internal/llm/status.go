// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm

import (
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// LLMConfigSource defines whether the effective LLM configuration resolves from BYOK, environment, or none.
type LLMConfigSource string

const (
	SourceBYOK LLMConfigSource = "byok"
	SourceEnv  LLMConfigSource = "env"
	SourceNone LLMConfigSource = "none"
)

// LLMModelCapabilities provides metadata on structured outputs and tool calling support.
type LLMModelCapabilities struct {
	StructuredOutput string `json:"structuredOutput,omitempty"`
	ToolCalling      string `json:"toolCalling,omitempty"`
}

// LLMModelStatus describes the status and resolvability of a configured model in an organization.
type LLMModelStatus struct {
	ModelID      string                `json:"modelId"`
	Model        string                `json:"model,omitempty"`
	ProviderID   string                `json:"providerId,omitempty"`
	BaseURL      string                `json:"baseUrl,omitempty"`
	Resolvable   bool                  `json:"resolvable"`
	Capabilities *LLMModelCapabilities `json:"capabilities,omitempty"`
}

// BYOKStatusSummary provides metadata on the active primary BYOK slot (secrets masked).
type BYOKStatusSummary struct {
	Configured bool   `json:"configured"`
	Model      string `json:"model,omitempty"`
	ProviderID string `json:"providerId,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty"`
}

// EnvStatusSummary describes the host system-managed environment LLM configuration.
type EnvStatusSummary struct {
	Configured          bool     `json:"configured"`
	Model               string   `json:"model,omitempty"`
	ProviderID          string   `json:"providerId,omitempty"`
	BaseURL             string   `json:"baseUrl,omitempty"`
	VertexLocation      string   `json:"vertexLocation,omitempty"`
	TemperatureOverride *float64 `json:"temperatureOverride,omitempty"`
}

// LLMConfigStatus is the per-org projected LLM configuration status.
type LLMConfigStatus struct {
	Source LLMConfigSource   `json:"source"`
	Models []LLMModelStatus  `json:"models"`
	BYOK   BYOKStatusSummary `json:"byok"`
	Env    EnvStatusSummary  `json:"env"`
}

// IsBYOKSlotConfigured determines whether a slot carries usable credentials to run inference.
func IsBYOKSlotConfigured(slot *byok.NormalizedModel) bool {
	return byok.IsBYOKSlotConfigured(slot)
}

// IsModelResolvable reports whether a configured model can actually execute.
func IsModelResolvable(model *byok.BYOKModelConfig, cred *byok.BYOKCredential, envReachable bool) bool {
	if model == nil || cred == nil {
		return false
	}
	if cred.IsManaged {
		return envReachable
	}
	if cred.Provider == "" || model.Model == "" {
		return false
	}

	slot := &byok.NormalizedModel{
		Provider: byok.BYOKProvider(cred.Provider),
		APIKey:   cred.APIKey,
	}
	if cred.Settings != nil {
		if t, ok := cred.Settings["awsBearerToken"].(string); ok {
			slot.AWSBearerToken = t
		}
		if id, ok := cred.Settings["awsAccessKeyId"].(string); ok {
			slot.AWSAccessKeyID = id
		}
		if sec, ok := cred.Settings["awsSecretAccessKey"].(string); ok {
			slot.AWSSecretAccessKey = sec
		}
	}

	return IsBYOKSlotConfigured(slot)
}

func getModelCapabilities(providerID, model string) *LLMModelCapabilities {
	if providerID == "" || model == "" || !kernel.DefaultRegistry.Has(providerID) {
		return nil
	}
	mod, ok := kernel.DefaultRegistry.Get(providerID)
	if !ok || mod == nil {
		return nil
	}
	caps := mod.Capabilities(model)
	return &LLMModelCapabilities{
		StructuredOutput: caps.StructuredOutput,
		ToolCalling:      caps.ToolCalling,
	}
}

// DescribeLLMConfigStatus inspects a stored BYOKConfig blob and returns the effective
// per-organization LLM status with secrets masked.
func DescribeLLMConfigStatus(config *byok.BYOKConfig) LLMConfigStatus {
	envConf := byok.LoadEnvLLMConfig()
	envReachable := envConf.HasManagedCredentials()

	envSummary := EnvStatusSummary{
		Configured:     envReachable,
		Model:          envConf.DefaultModel,
		ProviderID:     envConf.DefaultProvider,
		BaseURL:        envConf.OpenAIBaseURL,
		VertexLocation: envConf.VertexLocation,
	}

	byokSlot := byok.ResolveDefaultSlot(config)
	byokConfigured := IsBYOKSlotConfigured(byokSlot)

	byokSummary := BYOKStatusSummary{
		Configured: byokConfigured,
	}
	if byokConfigured && byokSlot != nil {
		byokSummary.Model = byokSlot.Model
		byokSummary.ProviderID = string(byokSlot.Provider)
		byokSummary.BaseURL = byokSlot.BaseURL
	}

	source := SourceNone
	if byokSummary.Configured {
		source = SourceBYOK
	} else if envSummary.Configured {
		source = SourceEnv
	}

	var models []LLMModelStatus
	if config != nil && len(config.Models) > 0 {
		credMap := make(map[string]*byok.BYOKCredential)
		for i := range config.Credentials {
			credMap[config.Credentials[i].ID] = &config.Credentials[i]
		}

		for _, m := range config.Models {
			cred := credMap[m.CredentialID]
			baseURL := ""
			var provID string
			if cred != nil {
				provID = cred.Provider
				if cred.Settings != nil {
					if u, ok := cred.Settings["baseURL"].(string); ok {
						baseURL = u
					}
				}
			}

			models = append(models, LLMModelStatus{
				ModelID:      m.ID,
				Model:        m.Model,
				ProviderID:   provID,
				BaseURL:      baseURL,
				Resolvable:   IsModelResolvable(&m, cred, envReachable),
				Capabilities: getModelCapabilities(provID, m.Model),
			})
		}
	}

	return LLMConfigStatus{
		Source: source,
		Models: models,
		BYOK:   byokSummary,
		Env:    envSummary,
	}
}
