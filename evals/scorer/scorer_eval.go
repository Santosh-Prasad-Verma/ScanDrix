// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scorer

import (
	"os"
	"strconv"
)

// ScorerWeights defines the relative contribution of each evaluation pillar.
type ScorerWeights struct {
	RecallWeight    float64 `json:"recall_weight"`
	PrecisionWeight float64 `json:"precision_weight"`
	FormatWeight    float64 `json:"format_weight"`
	AnchorWeight    float64 `json:"anchor_weight"`
}

// DefaultScorerWeights returns production-calibrated evaluation weights.
func DefaultScorerWeights() ScorerWeights {
	w := ScorerWeights{
		RecallWeight:    0.40,
		PrecisionWeight: 0.30,
		FormatWeight:    0.15,
		AnchorWeight:    0.15,
	}

	if val := os.Getenv("SCANDRIX_SCORE_RECALL_WEIGHT"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			w.RecallWeight = parsed
		}
	}
	if val := os.Getenv("SCANDRIX_SCORE_PRECISION_WEIGHT"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			w.PrecisionWeight = parsed
		}
	}
	if val := os.Getenv("SCANDRIX_SCORE_FORMAT_WEIGHT"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			w.FormatWeight = parsed
		}
	}
	if val := os.Getenv("SCANDRIX_SCORE_ANCHOR_WEIGHT"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			w.AnchorWeight = parsed
		}
	}

	return w
}

// CompositeScoreResult encapsulates multi-pillar quality metrics.
type CompositeScoreResult struct {
	TotalScore float64 `json:"total_score"`
	Recall     float64 `json:"recall"`
	Precision  float64 `json:"precision"`
	Format     float64 `json:"format"`
	Anchor     float64 `json:"anchor"`
	Passed     bool    `json:"passed"`
}

// ComputeCompositeScore calculates the weighted overall quality score for a review execution.
func ComputeCompositeScore(
	recall, precision, format, anchor float64,
	customWeights ...ScorerWeights,
) CompositeScoreResult {
	weights := DefaultScorerWeights()
	if len(customWeights) > 0 {
		weights = customWeights[0]
	}

	total := (recall * weights.RecallWeight) +
		(precision * weights.PrecisionWeight) +
		(format * weights.FormatWeight) +
		(anchor * weights.AnchorWeight)

	// Quality bar: >= 0.75 passes enterprise gate
	minPass := 0.75
	if val := os.Getenv("SCANDRIX_EVAL_GATE_THRESHOLD"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 {
			minPass = parsed
		}
	}

	return CompositeScoreResult{
		TotalScore: total,
		Recall:     recall,
		Precision:  precision,
		Format:     format,
		Anchor:     anchor,
		Passed:     total >= minPass,
	}
}
