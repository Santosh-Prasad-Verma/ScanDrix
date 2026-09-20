// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package reasoning_test

import (
	"testing"

	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/reasoning"
)

func TestBuildProviderOptions_AnthropicEffort(t *testing.T) {
	opts := reasoning.BuildProviderOptions(reasoning.ProviderOptionsInput{
		RunName:         "test-run",
		Provider:        "anthropic",
		ReasoningEffort: "medium",
	})

	anthropic, ok := opts["anthropic"].(map[string]any)
	if !ok {
		t.Fatal("expected anthropic namespace")
	}
	thinking, ok := anthropic["thinking"].(map[string]any)
	if !ok {
		t.Fatal("expected thinking config")
	}
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != 15000 {
		t.Fatalf("unexpected thinking payload: %v", thinking)
	}
}

func TestBuildProviderOptions_OpenRouterRouting(t *testing.T) {
	fb := false
	opts := reasoning.BuildProviderOptions(reasoning.ProviderOptionsInput{
		RunName:                  "test-run",
		Provider:                 "open_router",
		ReasoningEffort:          "low",
		OpenRouterProviderOrder:  []string{"Anthropic", "OpenAI"},
		OpenRouterAllowFallbacks: &fb,
	})

	or, ok := opts["openrouter"].(map[string]any)
	if !ok {
		t.Fatal("expected openrouter namespace")
	}
	prov, ok := or["provider"].(map[string]any)
	if !ok {
		t.Fatal("expected provider routing block")
	}
	order, ok := prov["order"].([]string)
	if !ok || len(order) != 2 {
		t.Fatalf("expected order [Anthropic, OpenAI], got %v", prov["order"])
	}
}

func TestBuildProviderOptions_JSONOverride(t *testing.T) {
	opts := reasoning.BuildProviderOptions(reasoning.ProviderOptionsInput{
		RunName:                 "test-run",
		Provider:                "openai",
		ReasoningConfigOverride: `{"reasoning_effort":"high"}`,
	})

	openai, ok := opts["openai"].(map[string]any)
	if !ok {
		t.Fatal("expected auto-wrapped openai namespace")
	}
	if openai["reasoning_effort"] != "high" {
		t.Fatalf("expected reasoning_effort high, got %v", openai)
	}
}
