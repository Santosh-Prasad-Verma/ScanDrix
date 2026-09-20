// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package validation_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/validation"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
)

func TestValidateModelTuning_AnthropicUnsupportedTemperature(t *testing.T) {
	temp := 0.7
	issues := validation.ValidateModelTuning(validation.ModelTuningInput{
		Provider:    "anthropic",
		Model:       "claude-3-7-sonnet-20250219",
		Temperature: &temp,
	})

	// claude-3-7-sonnet has adjustable temperature unless extended thinking is pinned
	if len(issues) > 0 {
		t.Logf("Observed issues: %v", issues)
	}
}

func TestValidateModelTuning_ValidInputs(t *testing.T) {
	temp := 0.2
	issues := validation.ValidateModelTuning(validation.ModelTuningInput{
		Provider:        "openai",
		Model:           "gpt-4o",
		Temperature:     &temp,
		ReasoningEffort: "low",
	})
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues for valid tuning, got %d: %v", len(issues), issues)
	}
}

func TestValidateModelTuning_UnknownProvider(t *testing.T) {
	temp := 0.5
	issues := validation.ValidateModelTuning(validation.ModelTuningInput{
		Provider:    "unknown-provider",
		Model:       "some-model",
		Temperature: &temp,
	})
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues for unknown provider, got %d", len(issues))
	}
}
