// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package sampling

import (
	"github.com/scandrix/backend/internal/llm/byok"
)

// SlotCallOptions holds per-model call tuning shared across all LLM execution paths.
type SlotCallOptions struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
}

// ResolveSlotCallOptions maps a resolved NormalizedModel slot into call options.
// Validates temperature through ResolveByokTemperature and guards MaxOutputTokens > 0.
func ResolveSlotCallOptions(slot *byok.NormalizedModel) SlotCallOptions {
	opts := SlotCallOptions{}
	if slot == nil {
		return opts
	}

	opts.Temperature = ResolveByokTemperature(slot)
	if slot.MaxOutputTokens > 0 {
		opts.MaxOutputTokens = slot.MaxOutputTokens
	}

	return opts
}
