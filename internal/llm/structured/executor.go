// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package structured

import (
	"fmt"
	"strings"
)

// StructuredPlan determines how structured output should be orchestrated.
type StructuredPlan string

const (
	// PlanAsIs executes normal structured output (response_format: json_schema or tools).
	PlanAsIs StructuredPlan = "as-is"

	// PlanSuppressThinking disables reasoning/thinking because the model 400s on forced tool choice with thinking.
	PlanSuppressThinking StructuredPlan = "suppress-thinking"

	// PlanRerouteJSON runs plain text generation with prompt-injected schema, keeping thinking on,
	// and extracts JSON via deterministic repair (used for always-thinking models like Kimi k2.7, Claude Mythos, GLM).
	PlanRerouteJSON StructuredPlan = "reroute-json"
)

// ResolveStructuredPlan decides the appropriate plan based on model capabilities.
func ResolveStructuredPlan(provider, model string, supportsThinking, thinksByDefault, forcedToolChoiceRejectsThinking bool) StructuredPlan {
	lowerModel := strings.ToLower(model)

	// Always-thinking models where tool_choice or json_schema with thinking is rejected
	if strings.Contains(lowerModel, "kimi-k2.7") ||
		strings.Contains(lowerModel, "kimi-k3") ||
		strings.Contains(lowerModel, "glm-5.3") ||
		strings.Contains(lowerModel, "claude-fable") ||
		strings.Contains(lowerModel, "claude-mythos") {
		return PlanRerouteJSON
	}

	if supportsThinking && forcedToolChoiceRejectsThinking {
		return PlanSuppressThinking
	}

	return PlanAsIs
}

// FormatPromptWithSchema injects the required JSON Schema contract into the system prompt.
func FormatPromptWithSchema(systemPrompt, schemaJSON string) string {
	if schemaJSON == "" {
		return systemPrompt
	}

	promptContract := fmt.Sprintf("Return ONLY a JSON object that conforms EXACTLY to this JSON Schema (same property names, no extra keys):\n%s", schemaJSON)
	if systemPrompt == "" {
		return promptContract
	}

	return fmt.Sprintf("%s\n\n%s", systemPrompt, promptContract)
}
