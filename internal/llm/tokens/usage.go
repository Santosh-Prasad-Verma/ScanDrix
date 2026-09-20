// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tokens

// TokenUsage encapsulates token consumption accounting reported by an LLM call
// used for cost attribution and observability spans.
type TokenUsage struct {
	InputTokens           int `json:"input_tokens,omitempty"`
	OutputTokens          int `json:"output_tokens,omitempty"`
	TotalTokens           int `json:"total_tokens,omitempty"`
	OutputReasoningTokens int `json:"output_reasoning_tokens,omitempty"`
	CacheReadTokens       int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens      int `json:"cache_write_tokens,omitempty"`

	// Observability metadata
	Model       string `json:"model,omitempty"`
	RunID       string `json:"runId,omitempty"`
	ParentRunID string `json:"parentRunId,omitempty"`
	RunName     string `json:"runName,omitempty"`
}
