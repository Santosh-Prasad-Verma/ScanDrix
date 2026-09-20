// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

// ProgressSummary captures the completion status of declared review targets.
type ProgressSummary struct {
	TotalTargets    int `json:"total_targets"`
	PendingTargets  int `json:"pending_targets"`
	CriticalTotal   int `json:"critical_total"`
	CriticalPending int `json:"critical_pending"`
}

// ProgressLedger tracks whether the agent investigated all declared critical targets
// (e.g. diff hunks, security endpoints) prior to finalizing.
type ProgressLedger interface {
	// MarkFromToolCall updates target coverage based on an executed tool call.
	MarkFromToolCall(toolName string, input any, step int)

	// Summary returns the current progress snapshot.
	Summary() ProgressSummary

	// DebtNote returns a human-readable guidance note of pending critical targets,
	// or nil if all critical targets have been addressed.
	DebtNote() *string
}
