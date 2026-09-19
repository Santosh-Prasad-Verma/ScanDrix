// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package enums

// FineTuningType indicates the scope of fine-tuning analysis.
type FineTuningType string

const (
	FineTuningTypeGlobal     FineTuningType = "global"
	FineTuningTypeRepository FineTuningType = "repository"
)
