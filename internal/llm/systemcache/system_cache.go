// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package systemcache

import (
	"strings"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// SystemCacheControlInput holds provider and model identifiers for resolving cache hints.
type SystemCacheControlInput struct {
	Provider string
	Model    string
}

// IsAnthropicModel detects whether a model string belongs to the Anthropic family.
func IsAnthropicModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "claude") || strings.Contains(m, "anthropic")
}

// SystemCacheControl resolves the system prompt cache hint for a slot.
// Attaching this hint allows multi-step agent loops and reviews to read long static
// prompts from cache rather than re-billing on every turn.
func SystemCacheControl(input SystemCacheControlInput) map[string]any {
	provider := strings.TrimSpace(input.Provider)
	model := strings.TrimSpace(input.Model)

	// 1. Provider known in registry -> let provider module own the protocol & shape
	if provider != "" && kernel.DefaultRegistry.Has(provider) {
		module, _ := kernel.DefaultRegistry.Get(provider)
		if module != nil {
			return module.SystemCacheControl(byok.NormalizedModel{
				Provider: byok.BYOKProvider(provider),
				Model:    model,
			})
		}
	}

	// 2. Provider unknown (env / managed default with no explicit slot) -> best-effort by name
	if IsAnthropicModel(model) {
		if kernel.DefaultRegistry.Has("anthropic") {
			module, _ := kernel.DefaultRegistry.Get("anthropic")
			if module != nil {
				return module.SystemCacheControl(byok.NormalizedModel{
					Provider: byok.ProviderAnthropic,
					Model:    model,
				})
			}
		}
		return map[string]any{
			"type": "ephemeral",
		}
	}

	return nil
}
