// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package kernel

import (
	"regexp"
)

var claudeOrAnthropicRegex = regexp.MustCompile(`(?i)claude|anthropic`)

// AnthropicEphemeralCacheHint constructs the ephemeral cache control marker map.
func AnthropicEphemeralCacheHint() map[string]any {
	return map[string]any{
		"anthropic": map[string]any{
			"cacheControl": map[string]any{
				"type": "ephemeral",
			},
		},
	}
}

// IsAnthropicModel reports whether a model name belongs to the Anthropic family.
func IsAnthropicModel(model string) bool {
	return claudeOrAnthropicRegex.MatchString(model)
}
