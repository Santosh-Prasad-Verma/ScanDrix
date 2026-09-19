// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"
)

// VerdictDimension represents a single rubric dimension evaluation.
type VerdictDimension struct {
	Name string `json:"name"`
	Pass bool   `json:"pass"`
	Note string `json:"note,omitempty"`
}

// VerifierToolCallRecord captures an individual tool call executed by the verifier.
type VerifierToolCallRecord struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args,omitempty"`
	Result string         `json:"result,omitempty"`
}

// Verdict is the structured judgment of a candidate output.
// Keep=true is the gate required to move a candidate into the kept set.
type Verdict struct {
	Keep       bool                     `json:"keep"`
	Confidence string                   `json:"confidence,omitempty"` // "high" | "medium" | "low"
	Rationale  string                   `json:"rationale,omitempty"`
	Dimensions []VerdictDimension       `json:"dimensions,omitempty"`
	ToolCalls  []VerifierToolCallRecord `json:"tool_calls,omitempty"`
}

// Verifier turns one candidate output of type T into an objective Verdict.
// Verifier implementations MUST fail-open (defaulting to keep=true on unexpected error)
// to prevent silent candidate drops caused by transient infrastructure faults.
type Verifier[T any] interface {
	Verify(ctx context.Context, candidate T, toolCtx ToolContext) (Verdict, error)
}
