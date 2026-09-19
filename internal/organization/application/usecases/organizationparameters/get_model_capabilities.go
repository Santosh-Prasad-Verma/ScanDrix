// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"strings"

	"github.com/scandrix/backend/internal/llm/contextwin"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ModelCapabilities maps capabilities for each model.
type ModelCapabilities struct {
	ModelID           string `json:"modelId"`
	SupportsTools     bool   `json:"supportsTools"`
	SupportsStreaming bool   `json:"supportsStreaming"`
	SupportsReasoning bool   `json:"supportsReasoning"`
	ContextWindow     int    `json:"contextWindow"`
	MaxOutputTokens   int    `json:"maxOutputTokens"`
}

// GetModelCapabilitiesUseCase returns capability profiles for AI models dynamically.
type GetModelCapabilitiesUseCase struct{}

func NewGetModelCapabilitiesUseCase() *GetModelCapabilitiesUseCase {
	return &GetModelCapabilitiesUseCase{}
}

func (uc *GetModelCapabilitiesUseCase) Execute(modelID string) ModelCapabilities {
	mID := strings.TrimSpace(modelID)
	contextWin := contextwin.GetModelContextWindow(mID)

	supportsReasoning := false
	supportsTools := true
	supportsStreaming := true
	maxOutput := 8192

	idLower := strings.ToLower(mID)
	if strings.Contains(idLower, "claude-3-7") || strings.Contains(idLower, "o1") || strings.Contains(idLower, "o3") || strings.Contains(idLower, "r1") {
		supportsReasoning = true
		maxOutput = 64000
	} else if strings.Contains(idLower, "gpt-4o") {
		maxOutput = 16384
	}

	// Check if registered provider module provides explicit capability metadata
	for _, mod := range kernel.List() {
		caps := mod.Capabilities(mID)
		if caps.MaxInputTokens > 0 {
			contextWin = caps.MaxInputTokens
		}
		if caps.SupportsReasoning {
			supportsReasoning = true
		}
		if caps.ToolCalling == "none" {
			supportsTools = false
		}
	}

	return ModelCapabilities{
		ModelID:           mID,
		SupportsTools:     supportsTools,
		SupportsStreaming: supportsStreaming,
		SupportsReasoning: supportsReasoning,
		ContextWindow:     contextWin,
		MaxOutputTokens:   maxOutput,
	}
}
