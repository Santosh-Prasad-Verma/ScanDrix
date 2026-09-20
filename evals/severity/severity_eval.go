// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package severity

import (
	"strings"
)

// SeverityLevel ranks finding impact.
type SeverityLevel string

const (
	SeverityCritical SeverityLevel = "critical"
	SeverityHigh     SeverityLevel = "high"
	SeverityMedium   SeverityLevel = "medium"
	SeverityLow      SeverityLevel = "low"
)

// SeverityEvaluationResult captures classification evaluation.
type SeverityEvaluationResult struct {
	Predicted   SeverityLevel
	Expected    SeverityLevel
	IsCorrect   bool
	IsWithinOne bool
	WeightDelta int
}

// SeverityWeight assigns ordinal rank for distance calculations.
func SeverityWeight(s SeverityLevel) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

// EvaluateSeverity judges model severity output against a validated benchmark finding.
func EvaluateSeverity(predicted, expected string) SeverityEvaluationResult {
	normPred := SeverityLevel(strings.ToLower(strings.TrimSpace(predicted)))
	normExp := SeverityLevel(strings.ToLower(strings.TrimSpace(expected)))

	wPred := SeverityWeight(normPred)
	wExp := SeverityWeight(normExp)
	delta := wPred - wExp
	if delta < 0 {
		delta = -delta
	}

	return SeverityEvaluationResult{
		Predicted:   normPred,
		Expected:    normExp,
		IsCorrect:   normPred == normExp,
		IsWithinOne: delta <= 1,
		WeightDelta: delta,
	}
}
