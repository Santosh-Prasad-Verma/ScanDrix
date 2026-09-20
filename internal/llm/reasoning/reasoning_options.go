// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package reasoning

import (
	"encoding/json"
	"strings"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// EffortToBudget maps reasoning effort presets to standard token budgets.
var EffortToBudget = map[string]int{
	"none":   0,
	"low":    5000,
	"medium": 15000,
	"high":   40000,
}

// DefaultReasoningEffortFor determines the default reasoning effort for a model slot.
// Derived from the provider module's intrinsic ReasoningTraits:
// Models that think by default receive "medium", while non-thinking models remain unset.
func DefaultReasoningEffortFor(slot *byok.NormalizedModel) string {
	if slot == nil {
		return ""
	}

	provider := string(slot.Provider)
	if provider == "" || !kernel.DefaultRegistry.Has(provider) {
		return ""
	}

	module, ok := kernel.DefaultRegistry.Get(provider)
	if !ok || module == nil {
		return ""
	}

	traits := module.ReasoningTraits(*slot)
	if traits.ThinksByDefault {
		return "medium"
	}

	return ""
}

// ProviderOptionsInput specifies parameters for building provider-specific options.
type ProviderOptionsInput struct {
	RunName                  string
	ReasoningEffort          string // "none" | "low" | "medium" | "high"
	ReasoningConfigOverride  string // Raw JSON override
	Provider                 string
	ModelName                string
	OpenRouterProviderOrder  []string
	OpenRouterAllowFallbacks *bool
}

// ProviderOptionsNamespace returns the wire namespace for provider options.
func ProviderOptionsNamespace(provider string) string {
	p := strings.ToLower(provider)
	switch p {
	case "anthropic", "anthropic_compatible":
		return "anthropic"
	case "openai", "openai_compatible":
		return "openai"
	case "google_gemini", "gemini":
		return "google"
	case "google_vertex", "vertex":
		return "vertex"
	case "amazon_bedrock", "bedrock":
		return "bedrock"
	case "open_router", "openrouter":
		return "openrouter"
	default:
		return p
	}
}

// AutoWrapProviderOverride wraps an un-namespaced JSON override under the provider's namespace.
func AutoWrapProviderOverride(override map[string]any, provider string) map[string]any {
	if len(override) == 0 {
		return override
	}

	ns := ProviderOptionsNamespace(provider)
	if ns == "" {
		return override
	}

	// If already namespaced under a known provider or langsmith, pass through
	for k := range override {
		if k == "anthropic" || k == "openai" || k == "google" || k == "openrouter" || k == "langsmith" {
			return override
		}
	}

	return map[string]any{
		ns: override,
	}
}

// BuildOpenRouterRouting constructs the OpenRouter provider pinning payload if configured.
func BuildOpenRouterRouting(input ProviderOptionsInput) map[string]any {
	p := strings.ToLower(input.Provider)
	if p != "open_router" && p != "openrouter" {
		return nil
	}

	var validOrder []string
	for _, ord := range input.OpenRouterProviderOrder {
		ord = strings.TrimSpace(ord)
		if ord != "" {
			validOrder = append(validOrder, ord)
		}
	}

	hasOrder := len(validOrder) > 0
	hasFallbacks := input.OpenRouterAllowFallbacks != nil

	if !hasOrder && !hasFallbacks {
		return nil
	}

	providerPayload := make(map[string]any)
	if hasOrder {
		providerPayload["order"] = validOrder
	}
	if hasFallbacks {
		providerPayload["allow_fallbacks"] = *input.OpenRouterAllowFallbacks
	}

	return map[string]any{
		"openrouter": map[string]any{
			"provider": providerPayload,
		},
	}
}

// BuildProviderOptions constructs the combined providerOptions payload for LLM calls.
func BuildProviderOptions(input ProviderOptionsInput) map[string]any {
	result := make(map[string]any)

	// 1. Check for raw JSON override
	if strings.TrimSpace(input.ReasoningConfigOverride) != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(input.ReasoningConfigOverride), &parsed); err == nil {
			wrapped := AutoWrapProviderOverride(parsed, input.Provider)
			for k, v := range wrapped {
				result[k] = v
			}
			// Layer OpenRouter routing on top if present
			if routing := BuildOpenRouterRouting(input); routing != nil {
				for k, v := range routing {
					result[k] = v
				}
			}
			return result
		}
	}

	// 2. Effort-based thinking options
	effort := strings.ToLower(strings.TrimSpace(input.ReasoningEffort))
	if effort != "" && effort != "none" {
		ns := ProviderOptionsNamespace(input.Provider)
		budget := EffortToBudget[effort]

		switch ns {
		case "anthropic":
			result["anthropic"] = map[string]any{
				"thinking": map[string]any{
					"type":          "enabled",
					"budget_tokens": budget,
				},
			}
		case "openai":
			result["openai"] = map[string]any{
				"reasoning_effort": effort,
			}
		case "openrouter":
			result["openrouter"] = map[string]any{
				"reasoning": map[string]any{
					"effort": effort,
				},
			}
		}
	}

	// 3. Layer OpenRouter provider pinning
	if routing := BuildOpenRouterRouting(input); routing != nil {
		if existing, ok := result["openrouter"].(map[string]any); ok {
			for k, v := range routing["openrouter"].(map[string]any) {
				existing[k] = v
			}
		} else {
			for k, v := range routing {
				result[k] = v
			}
		}
	}

	return result
}
