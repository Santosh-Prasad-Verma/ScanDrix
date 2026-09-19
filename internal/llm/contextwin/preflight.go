// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contextwin

import "math"

const (
	// PreflightCharsPerToken is the conservative ratio used for token estimation.
	PreflightCharsPerToken = 4.0

	// PreflightOutputReserveRatio is the fraction of context window held back for reasoning & output.
	PreflightOutputReserveRatio = 0.15

	// PreflightMinOutputReserveTokens is the minimum reserved token budget.
	PreflightMinOutputReserveTokens = 2048
)

// AssertPromptFitsInContext estimates prompt tokens and refuses to proceed if they exceed the context window.
func AssertPromptFitsInContext(systemPrompt, userPrompt string, contextWindowTokens int, modelName string) error {
	if contextWindowTokens <= 0 {
		return nil
	}

	promptChars := len(systemPrompt) + len(userPrompt)
	estimatedTokens := int(math.Ceil(float64(promptChars) / PreflightCharsPerToken))

	outputReserve := int(math.Floor(float64(contextWindowTokens) * PreflightOutputReserveRatio))
	if outputReserve < PreflightMinOutputReserveTokens {
		outputReserve = PreflightMinOutputReserveTokens
	}

	if estimatedTokens+outputReserve > contextWindowTokens {
		return &PromptTooLargeError{
			EstimatedTokens:     estimatedTokens,
			ContextWindowTokens: contextWindowTokens,
			ModelName:           modelName,
		}
	}

	return nil
}

// AssertPromptOverheadFits checks whether static overhead alone exceeds the context window.
func AssertPromptOverheadFits(systemPrompt, toolSchemasJSON string, contextWindowTokens int, modelName string) error {
	if contextWindowTokens <= 0 {
		return nil
	}

	overheadChars := len(systemPrompt) + len(toolSchemasJSON)
	overheadTokens := int(math.Ceil(float64(overheadChars) / PreflightCharsPerToken))

	if overheadTokens >= contextWindowTokens {
		return &ContextWindowTooSmallError{
			ContextWindow:  contextWindowTokens,
			OverheadTokens: overheadTokens,
			ModelName:      modelName,
		}
	}

	return nil
}
