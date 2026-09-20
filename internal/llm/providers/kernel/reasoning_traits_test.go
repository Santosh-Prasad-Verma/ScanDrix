// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package kernel

import (
	"testing"
)

func TestPlanStructuredCall(t *testing.T) {
	// 1. Response format modes (json_schema / json_object) are always fine (as-is)
	for _, mode := range []string{"json_schema", "json_object"} {
		tr := NonReasoningTraits
		tr.ThinksByDefault = true
		tr.RejectsThinkingWhenToolsForced = true
		if plan := PlanStructuredCall(mode, tr); plan != PlanAsIs {
			t.Errorf("expected as-is for mode %s, got %s", mode, plan)
		}
	}

	// 2. Tool-use + provider that can't force tool_choice (GLM) -> reroute-json
	trGLM := NonReasoningTraits
	trGLM.ForcedToolChoiceSupported = false
	if plan := PlanStructuredCall("none", trGLM); plan != PlanRerouteJSON {
		t.Errorf("expected reroute-json for GLM without forced tool choice, got %s", plan)
	}

	// 3. Tool-use + forced tool_choice does NOT reject thinking (DeepSeek) -> as-is
	trDeepSeek := NonReasoningTraits
	trDeepSeek.ThinksByDefault = true
	trDeepSeek.RejectsThinkingWhenToolsForced = false
	if plan := PlanStructuredCall("none", trDeepSeek); plan != PlanAsIs {
		t.Errorf("expected as-is for DeepSeek, got %s", plan)
	}

	// 4. Tool-use + rejects thinking + CAN disable (Kimi k2.6) -> suppress-thinking
	trKimi := NonReasoningTraits
	trKimi.ThinksByDefault = true
	trKimi.RejectsThinkingWhenToolsForced = true
	trKimi.CanDisableThinking = true
	if plan := PlanStructuredCall("none", trKimi); plan != PlanSuppressThinking {
		t.Errorf("expected suppress-thinking for Kimi k2.6, got %s", plan)
	}

	// 5. Tool-use + rejects thinking + CANNOT disable (k3) -> reroute-json
	trK3 := NonReasoningTraits
	trK3.ThinksByDefault = true
	trK3.RejectsThinkingWhenToolsForced = true
	trK3.CanDisableThinking = false
	if plan := PlanStructuredCall("none", trK3); plan != PlanRerouteJSON {
		t.Errorf("expected reroute-json for always-thinking K3, got %s", plan)
	}

	// 6. Non-reasoning default is always as-is
	if plan := PlanStructuredCall("none", NonReasoningTraits); plan != PlanAsIs {
		t.Errorf("expected as-is for non-reasoning traits, got %s", plan)
	}
}

func TestResolveCompatibleReasoningTraits(t *testing.T) {
	glm := ResolveCompatibleReasoningTraits("glm-4")
	if !glm.ThinksByDefault || glm.ForcedToolChoiceSupported {
		t.Errorf("unexpected traits for GLM: %+v", glm)
	}

	kimi := ResolveCompatibleReasoningTraits("moonshot-v1-8k")
	if !kimi.ThinksByDefault || !kimi.ForcedToolChoiceSupported {
		t.Errorf("unexpected traits for Kimi: %+v", kimi)
	}

	deepseek := ResolveCompatibleReasoningTraits("deepseek-reasoner")
	if !deepseek.ThinksByDefault || deepseek.RejectsThinkingWhenToolsForced {
		t.Errorf("unexpected traits for DeepSeek: %+v", deepseek)
	}
}

func TestAnthropicCacheHelpers(t *testing.T) {
	if !IsAnthropicModel("claude-3-7-sonnet-20250219") {
		t.Errorf("expected true for Claude model")
	}
	if !IsAnthropicModel("us.anthropic.claude-3-5-sonnet") {
		t.Errorf("expected true for Bedrock Claude model")
	}
	if IsAnthropicModel("gpt-4o") {
		t.Errorf("expected false for GPT model")
	}

	hint := AnthropicEphemeralCacheHint()
	if hint["anthropic"] == nil {
		t.Errorf("expected anthropic key in cache hint")
	}
}
