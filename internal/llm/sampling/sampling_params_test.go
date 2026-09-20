// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package sampling_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/sampling"
)

func TestResolveByokTemperature(t *testing.T) {
	temp := 0.7
	slot := &byok.NormalizedModel{
		Provider:    byok.ProviderOpenAI,
		Model:       "gpt-4o",
		Temperature: &temp,
	}

	resolved := sampling.ResolveByokTemperature(slot)
	if resolved == nil || *resolved != 0.7 {
		t.Fatalf("expected 0.7, got %v", resolved)
	}

	// Slot options
	opts := sampling.ResolveSlotCallOptions(slot)
	if opts.Temperature == nil || *opts.Temperature != 0.7 {
		t.Fatalf("expected opts.Temperature == 0.7, got %v", opts.Temperature)
	}
}

func TestResolveSlotCallOptions_MaxTokens(t *testing.T) {
	slot := &byok.NormalizedModel{
		Provider:        byok.ProviderAnthropic,
		Model:           "claude-3-7-sonnet-20250219",
		MaxOutputTokens: 4096,
	}

	opts := sampling.ResolveSlotCallOptions(slot)
	if opts.MaxOutputTokens != 4096 {
		t.Fatalf("expected 4096 max tokens, got %d", opts.MaxOutputTokens)
	}

	// Negative/zero max tokens should be omitted (0)
	slot.MaxOutputTokens = -1
	optsZero := sampling.ResolveSlotCallOptions(slot)
	if optsZero.MaxOutputTokens != 0 {
		t.Fatalf("expected 0 max tokens, got %d", optsZero.MaxOutputTokens)
	}
}
