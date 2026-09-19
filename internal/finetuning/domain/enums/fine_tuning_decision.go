// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package enums

// FineTuningDecision represents the retention decision for an AI code suggestion.
type FineTuningDecision string

const (
	DecisionKeep      FineTuningDecision = "KEEP"
	DecisionDiscard   FineTuningDecision = "DISCARD"
	DecisionUncertain FineTuningDecision = "UNCERTAIN"
)
