// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tokens

import "math"

// FallbackCharsPerToken is the conservative ratio used when tokenizing code and JSON.
// Dense code averages ~3 chars per token, providing a safe upper bound.
const FallbackCharsPerToken = 3

// EstimateTextTokens calculates an estimated token count for a text payload.
// Uses conservative bounds to ensure reliable pre-call rate-limit budgeting.
func EstimateTextTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / float64(FallbackCharsPerToken)))
}
