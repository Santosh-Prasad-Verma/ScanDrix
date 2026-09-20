// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contextwin

import "fmt"

// ContextWindowTooSmallError is returned when the agent's static prompt overhead alone exceeds the model's context window.
type ContextWindowTooSmallError struct {
	ContextWindow  int
	OverheadTokens int
	ModelName      string
}

func (e *ContextWindowTooSmallError) Error() string {
	return fmt.Sprintf("model '%s' has a context length of %d tokens, but static prompt overhead alone is %d tokens. Choose a model with larger context",
		e.ModelName, e.ContextWindow, e.OverheadTokens)
}

// PromptTooLargeError is returned when the estimated prompt exceeds the model's context window.
type PromptTooLargeError struct {
	EstimatedTokens     int
	ContextWindowTokens int
	ModelName           string
}

func (e *PromptTooLargeError) Error() string {
	return fmt.Sprintf("estimated prompt of %d tokens exceeds maximum context length of %d tokens for model '%s'",
		e.EstimatedTokens, e.ContextWindowTokens, e.ModelName)
}
