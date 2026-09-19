// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package kernel

import (
	"strings"
)

// NonReasoningTraits provides safe default traits for non-reasoning or unknown models.
var NonReasoningTraits = ModelReasoningTraits{
	ThinksByDefault:                false,
	CanDisableThinking:             true,
	ForcedToolChoiceSupported:      true,
	RejectsThinkingWhenToolsForced: false,
	BudgetMode:                     "none",
}

// ResolveCompatibleReasoningTraits extracts model reasoning facts for Anthropic-compatible endpoints.
func ResolveCompatibleReasoningTraits(model string) ModelReasoningTraits {
	m := strings.ToLower(model)

	// GLM (Z.ai): supports tool_choice "auto" only; GLM-5.3 forces thinking on.
	if strings.Contains(m, "glm") {
		alwaysThinking := strings.Contains(m, "5.3") || strings.Contains(m, "5-3")
		return ModelReasoningTraits{
			ThinksByDefault:                true,
			CanDisableThinking:             !alwaysThinking,
			ForcedToolChoiceSupported:      false,
			RejectsThinkingWhenToolsForced: true,
			BudgetMode:                     "effort_only",
		}
	}

	// Kimi (Moonshot): k2.7-code and k3 think always; k2.5/k2.6 can be disabled.
	if strings.Contains(m, "kimi") || strings.Contains(m, "moonshot") {
		alwaysThinking := strings.Contains(m, "code") || strings.Contains(m, "k3")
		return ModelReasoningTraits{
			ThinksByDefault:                true,
			CanDisableThinking:             !alwaysThinking,
			ForcedToolChoiceSupported:      true,
			RejectsThinkingWhenToolsForced: true,
			BudgetMode:                     "range",
		}
	}

	// DeepSeek: thinks by default, can disable, accepts forced tool choice with thinking.
	if strings.Contains(m, "deepseek") {
		return ModelReasoningTraits{
			ThinksByDefault:                true,
			CanDisableThinking:             true,
			ForcedToolChoiceSupported:      true,
			RejectsThinkingWhenToolsForced: false,
			BudgetMode:                     "effort_only",
		}
	}

	return NonReasoningTraits
}

// CompatibleTemperaturePolicy returns the temperature policy for compatible-protocol models.
func CompatibleTemperaturePolicy(model string) *TemperaturePolicy {
	t := ResolveCompatibleReasoningTraits(model)
	if t.ThinksByDefault && !t.CanDisableThinking {
		one := 1.0
		return &TemperaturePolicy{
			Mode:       TemperatureFixed,
			Kind:       "fixed",
			FixedValue: &one,
		}
	}
	return &TemperaturePolicy{
		Mode: TemperatureFree,
		Kind: "adjustable",
	}
}

// StructuredCallPlan defines what a structured call must do for a model.
type StructuredCallPlan string

const (
	PlanAsIs             StructuredCallPlan = "as-is"
	PlanSuppressThinking StructuredCallPlan = "suppress-thinking"
	PlanRerouteJSON      StructuredCallPlan = "reroute-json"
)

// PlanStructuredCall determines how structured output should be executed given model capability and traits.
func PlanStructuredCall(structuredOutput string, traits ModelReasoningTraits) StructuredCallPlan {
	if structuredOutput != "" && structuredOutput != "none" {
		return PlanAsIs
	}

	if !traits.ForcedToolChoiceSupported {
		return PlanRerouteJSON
	}
	if !traits.RejectsThinkingWhenToolsForced {
		return PlanAsIs
	}
	if traits.CanDisableThinking {
		return PlanSuppressThinking
	}
	return PlanRerouteJSON
}
