// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package prsummary

import (
	"strings"
)

// SummaryEvaluationResult evaluates generated PR summaries.
type SummaryEvaluationResult struct {
	HasHeadline       bool    `json:"has_headline"`
	HasWalkthrough    bool    `json:"has_walkthrough"`
	HasChangedFiles   bool    `json:"has_changed_files"`
	HasRiskAssessment bool    `json:"has_risk_assessment"`
	Score             float64 `json:"score"`
	Passed            bool    `json:"passed"`
}

// EvaluatePRSummary scores a generated PR summary against standard structural requirements:
// 1. Headline (short, punchy description)
// 2. Walkthrough (detailed explanations of changes)
// 3. File modifications overview
// 4. Risk / breaking change assessment
func EvaluatePRSummary(summaryText string) SummaryEvaluationResult {
	lower := strings.ToLower(summaryText)

	hasHeadline := len(strings.TrimSpace(summaryText)) > 20 &&
		(strings.Contains(lower, "summary") || strings.Contains(lower, "overview") || strings.Contains(summaryText, "#"))

	hasWalkthrough := strings.Contains(lower, "walkthrough") ||
		strings.Contains(lower, "changes") ||
		strings.Contains(lower, "key changes") ||
		strings.Contains(lower, "details")

	hasChangedFiles := strings.Contains(lower, ".go") ||
		strings.Contains(lower, ".ts") ||
		strings.Contains(lower, ".py") ||
		strings.Contains(lower, "file") ||
		strings.Contains(lower, "files")

	hasRisk := strings.Contains(lower, "risk") ||
		strings.Contains(lower, "breaking") ||
		strings.Contains(lower, "impact") ||
		strings.Contains(lower, "security") ||
		strings.Contains(lower, "verification")

	points := 0.0
	if hasHeadline {
		points += 0.25
	}
	if hasWalkthrough {
		points += 0.35
	}
	if hasChangedFiles {
		points += 0.20
	}
	if hasRisk {
		points += 0.20
	}

	return SummaryEvaluationResult{
		HasHeadline:       hasHeadline,
		HasWalkthrough:    hasWalkthrough,
		HasChangedFiles:   hasChangedFiles,
		HasRiskAssessment: hasRisk,
		Score:             points,
		Passed:            points >= 0.70,
	}
}
