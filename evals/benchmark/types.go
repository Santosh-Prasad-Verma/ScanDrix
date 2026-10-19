// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Golden Benchmark Evaluation Engine (SDRX-Bench)
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package benchmark

import (
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// AnnotatedDefect represents a human-annotated ground-truth defect in a benchmark PR.
type AnnotatedDefect struct {
	DefectID    string `json:"defect_id"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Category    string `json:"category"` // security, bug, performance, concurrency, resource_leak
	Severity    string `json:"severity"` // CRITICAL, HIGH, MEDIUM, LOW
	Description string `json:"description"`
}

// BenchmarkCase represents a single real-world pull request scenario with ground-truth defect labels.
type BenchmarkCase struct {
	ID                string            `json:"id"`
	Repository        string            `json:"repository"`
	PullRequestNumber int               `json:"pull_request_number"`
	Language          string            `json:"language"`
	Title             string            `json:"title"`
	DiffContent       string            `json:"diff_content"`
	AnnotatedDefects  []AnnotatedDefect `json:"annotated_defects"`
}

// BenchmarkEvaluationResult captures precision, recall, and F1 for a single benchmark case.
type BenchmarkEvaluationResult struct {
	CaseID         string        `json:"case_id"`
	Repository     string        `json:"repository"`
	Language       string        `json:"language"`
	TruePositives  int           `json:"true_positives"`
	FalsePositives int           `json:"false_positives"`
	FalseNegatives int           `json:"false_negatives"`
	Precision      float64       `json:"precision"`
	Recall         float64       `json:"recall"`
	F1Score        float64       `json:"f1_score"`
	Latency        time.Duration `json:"latency"`
}

// LanguageMetrics provides breakdown metrics for a specific programming language.
type LanguageMetrics struct {
	Cases          int     `json:"cases"`
	TotalDefects   int     `json:"total_defects"`
	TruePositives  int     `json:"true_positives"`
	FalsePositives int     `json:"false_positives"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	F1Score        float64 `json:"f1_score"`
}

// BenchmarkSuiteReport aggregates precision, recall, and F1 across all benchmark repositories.
type BenchmarkSuiteReport struct {
	TotalCases              int                        `json:"total_cases"`
	TotalGroundTruthDefects int                        `json:"total_ground_truth_defects"`
	TotalDetectedFindings   int                        `json:"total_detected_findings"`
	MacroPrecision          float64                    `json:"macro_precision"`
	MacroRecall             float64                    `json:"macro_recall"`
	MacroF1Score            float64                    `json:"macro_f1_score"`
	ResultsByLanguage       map[string]LanguageMetrics `json:"results_by_language"`
	CaseResults             []BenchmarkEvaluationResult `json:"case_results"`
	Duration                time.Duration              `json:"duration"`
}

// ModelFindingAdapter provides a standard interface to adapt findings from any internal ScanDrix model.
type ModelFindingAdapter struct {
	FilePath  string
	StartLine int
	EndLine   int
	Category  string
	Severity  models.FindingSeverity
	Title     string
}
