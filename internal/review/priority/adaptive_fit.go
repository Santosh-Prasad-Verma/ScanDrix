// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority


// AdaptiveProfileKind classifies the fidelity profile based on model context window constraints.
type AdaptiveProfileKind string

const (
	ProfileFull      AdaptiveProfileKind = "full"
	ProfileLight     AdaptiveProfileKind = "light"
	ProfileCompact   AdaptiveProfileKind = "compact"
	ProfileMinimal   AdaptiveProfileKind = "minimal"
	ProfileUnviable  AdaptiveProfileKind = "unviable"
)

// AdaptiveProfile encapsulates operational flags for context-constrained models.
type AdaptiveProfile struct {
	Kind                         AdaptiveProfileKind `json:"kind"`
	ContextWindowTokens          int                 `json:"context_window_tokens"`
	CompactPrompt                bool                `json:"compact_prompt"`
	DropCallGraph                bool                `json:"drop_call_graph"`
	AllOptional                  bool                `json:"all_optional"`
	MaxDiffChars                 *int                `json:"max_diff_chars,omitempty"`
	SkipHeavyPasses              bool                `json:"skip_heavy_passes"`
	LowSignalFilterUnconditional bool                `json:"low_signal_filter_unconditional"`
}

const (
	FullThreshold    = 64000
	LightThreshold   = 32000
	CompactThreshold = 16000
	MinimalThreshold = 8000

	MinimalProfileMaxDiffChars = 4000
)

func classifyProfile(tokens int) AdaptiveProfileKind {
	if tokens <= 0 {
		return ProfileUnviable
	}
	if tokens >= FullThreshold {
		return ProfileFull
	}
	if tokens >= LightThreshold {
		return ProfileLight
	}
	if tokens >= CompactThreshold {
		return ProfileCompact
	}
	if tokens >= MinimalThreshold {
		return ProfileMinimal
	}
	return ProfileUnviable
}

// ResolveAdaptiveProfile determines operational flags based on token headroom.
func ResolveAdaptiveProfile(contextWindowTokens int) AdaptiveProfile {
	kind := classifyProfile(contextWindowTokens)
	resolvedWindow := contextWindowTokens
	if resolvedWindow < 0 {
		resolvedWindow = 0
	}

	light := kind == ProfileLight || kind == ProfileCompact || kind == ProfileMinimal
	compact := kind == ProfileCompact || kind == ProfileMinimal
	minimal := kind == ProfileMinimal

	var maxChars *int
	if minimal {
		c := MinimalProfileMaxDiffChars
		maxChars = &c
	}

	return AdaptiveProfile{
		Kind:                         kind,
		ContextWindowTokens:          resolvedWindow,
		DropCallGraph:                light,
		SkipHeavyPasses:              light,
		CompactPrompt:                compact,
		LowSignalFilterUnconditional: compact,
		AllOptional:                  minimal,
		MaxDiffChars:                 maxChars,
	}
}
