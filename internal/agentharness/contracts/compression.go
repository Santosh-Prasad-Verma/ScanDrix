// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

// CompressionResult represents the outcome of a message window compression pass.
type CompressionResult struct {
	Messages     []AgentMessage
	BeforeTokens int
	AfterTokens  int
}

// Compressor provides context window compaction strategies to prevent context overflow.
type Compressor interface {
	// MaybeCompress evaluates the message window and returns a compressed window
	// if token savings can be achieved, or nil if no compression is needed.
	MaybeCompress(messages []AgentMessage) *CompressionResult
}
