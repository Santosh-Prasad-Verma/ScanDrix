// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Golden Benchmark Evaluation Engine (SDRX-Bench)
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package benchmark

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

func TestLoadBenchmarkCases(t *testing.T) {
	datasetPath := filepath.Join("datasets", "sdrx_golden_benchmark.json")
	cases, err := LoadBenchmarkCases(datasetPath)
	if err != nil {
		t.Fatalf("LoadBenchmarkCases failed: %v", err)
	}

	if len(cases) != 4 {
		t.Fatalf("expected 4 benchmark cases, got %d", len(cases))
	}

	langSet := make(map[string]bool)
	for _, c := range cases {
		langSet[c.Language] = true
	}

	expectedLangs := []string{"go", "python", "typescript", "rust"}
	for _, l := range expectedLangs {
		if !langSet[l] {
			t.Errorf("missing expected language %q in benchmark dataset", l)
		}
	}
}

func TestEvaluateCase_PrecisionRecallF1(t *testing.T) {
	bCase := BenchmarkCase{
		ID:         "test-case-1",
		Repository: "scandrix/test-repo",
		Language:   "go",
		AnnotatedDefects: []AnnotatedDefect{
			{
				DefectID:   "d1",
				FilePath:   "pkg/auth/token.go",
				LineNumber: 42,
				Category:   "security",
				Severity:   "CRITICAL",
			},
			{
				DefectID:   "d2",
				FilePath:   "pkg/auth/token.go",
				LineNumber: 88,
				Category:   "bug",
				Severity:   "HIGH",
			},
		},
	}

	// Simulated findings:
	// finding 1: matches d1 exactly on line 42 (TP)
	// finding 2: matches d2 within tolerance on line 89 (offset 1 <= 3) (TP)
	// finding 3: spurious finding on line 120 (FP)
	findings := []ModelFindingAdapter{
		{
			FilePath:  "pkg/auth/token.go",
			StartLine: 42,
			Category:  "security",
			Severity:  models.SeverityCritical,
			Title:     "SQL Injection in token generator",
		},
		{
			FilePath:  "pkg/auth/token.go",
			StartLine: 89,
			Category:  "bug",
			Severity:  models.SeverityHigh,
			Title:     "Potential nil pointer dereference",
		},
		{
			FilePath:  "pkg/auth/token.go",
			StartLine: 120,
			Category:  "style",
			Severity:  models.SeverityLow,
			Title:     "Consider renaming variable",
		},
	}

	res := EvaluateCase(bCase, findings, DefaultLineTolerance)

	if res.TruePositives != 2 {
		t.Errorf("expected 2 True Positives, got %d", res.TruePositives)
	}
	if res.FalsePositives != 1 {
		t.Errorf("expected 1 False Positive, got %d", res.FalsePositives)
	}
	if res.FalseNegatives != 0 {
		t.Errorf("expected 0 False Negatives, got %d", res.FalseNegatives)
	}

	// Precision = 2 / (2 + 1) = 0.666...
	expectedPrecision := 2.0 / 3.0
	if diff := res.Precision - expectedPrecision; diff > 0.001 || diff < -0.001 {
		t.Errorf("precision mismatch: got %.4f, want %.4f", res.Precision, expectedPrecision)
	}

	// Recall = 2 / (2 + 0) = 1.0
	if res.Recall != 1.0 {
		t.Errorf("recall mismatch: got %.4f, want 1.0", res.Recall)
	}

	// F1 = 2 * ( (2/3) * 1 ) / ( (2/3) + 1 ) = (4/3) / (5/3) = 0.8
	expectedF1 := 0.8
	if diff := res.F1Score - expectedF1; diff > 0.001 || diff < -0.001 {
		t.Errorf("F1 score mismatch: got %.4f, want %.4f", res.F1Score, expectedF1)
	}
}

func TestAggregateSuite(t *testing.T) {
	results := []BenchmarkEvaluationResult{
		{
			CaseID:         "c1",
			Language:       "go",
			TruePositives:  3,
			FalsePositives: 1,
			FalseNegatives: 0,
			Precision:      0.75,
			Recall:         1.0,
			F1Score:        0.857,
		},
		{
			CaseID:         "c2",
			Language:       "typescript",
			TruePositives:  2,
			FalsePositives: 0,
			FalseNegatives: 1,
			Precision:      1.0,
			Recall:         0.666,
			F1Score:        0.8,
		},
	}

	report := AggregateSuite(results, 250*time.Millisecond)

	if report.TotalCases != 2 {
		t.Errorf("expected 2 total cases, got %d", report.TotalCases)
	}
	if report.TotalGroundTruthDefects != 6 { // (3+0) + (2+1) = 6
		t.Errorf("expected 6 ground truth defects, got %d", report.TotalGroundTruthDefects)
	}
	if report.TotalDetectedFindings != 6 { // (3+1) + (2+0) = 6
		t.Errorf("expected 6 detected findings, got %d", report.TotalDetectedFindings)
	}

	if _, ok := report.ResultsByLanguage["go"]; !ok {
		t.Errorf("missing go metrics in language breakdown")
	}
	if _, ok := report.ResultsByLanguage["typescript"]; !ok {
		t.Errorf("missing typescript metrics in language breakdown")
	}
}
