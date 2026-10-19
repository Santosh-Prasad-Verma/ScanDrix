// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Golden Benchmark Evaluation Engine (SDRX-Bench)
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultLineTolerance defines allowable line offset between ground truth and detection (+/- 3 lines).
const DefaultLineTolerance = 3

// EvaluateCase matches detected findings against annotated ground-truth defects for a single case.
func EvaluateCase(bCase BenchmarkCase, findings []ModelFindingAdapter, lineTolerance int) BenchmarkEvaluationResult {
	if lineTolerance <= 0 {
		lineTolerance = DefaultLineTolerance
	}

	annotatedCount := len(bCase.AnnotatedDefects)
	matchedDefects := make(map[string]bool)
	matchedFindings := make(map[int]bool)

	for defectIdx, defect := range bCase.AnnotatedDefects {
		cleanDefectPath := filepath.Clean(filepath.ToSlash(defect.FilePath))

		for findingIdx, finding := range findings {
			if matchedFindings[findingIdx] {
				continue
			}

			cleanFindingPath := filepath.Clean(filepath.ToSlash(finding.FilePath))

			// Paths must match by suffix or exact equality
			if cleanFindingPath != cleanDefectPath &&
				!strings.HasSuffix(cleanFindingPath, cleanDefectPath) &&
				!strings.HasSuffix(cleanDefectPath, cleanFindingPath) {
				continue
			}

			// Line tolerance check
			lineDiff := int(math.Abs(float64(finding.StartLine - defect.LineNumber)))
			if lineDiff <= lineTolerance {
				matchedDefects[defect.DefectID] = true
				matchedFindings[findingIdx] = true
				_ = defectIdx
				break
			}
		}
	}

	tp := len(matchedDefects)
	fp := len(findings) - len(matchedFindings)
	if fp < 0 {
		fp = 0
	}
	fn := annotatedCount - tp
	if fn < 0 {
		fn = 0
	}

	var precision, recall, f1 float64
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	if precision+recall > 0 {
		f1 = 2 * (precision * recall) / (precision + recall)
	}

	return BenchmarkEvaluationResult{
		CaseID:         bCase.ID,
		Repository:     bCase.Repository,
		Language:       bCase.Language,
		TruePositives:  tp,
		FalsePositives: fp,
		FalseNegatives: fn,
		Precision:      precision,
		Recall:         recall,
		F1Score:        f1,
	}
}

// AggregateSuite aggregates results across all benchmark cases and computes macro metrics.
func AggregateSuite(results []BenchmarkEvaluationResult, duration time.Duration) BenchmarkSuiteReport {
	report := BenchmarkSuiteReport{
		TotalCases:        len(results),
		ResultsByLanguage: make(map[string]LanguageMetrics),
		CaseResults:       results,
		Duration:          duration,
	}

	if len(results) == 0 {
		return report
	}

	var totalTP, totalFP, totalFN int
	var sumPrecision, sumRecall, sumF1 float64

	for _, res := range results {
		totalTP += res.TruePositives
		totalFP += res.FalsePositives
		totalFN += res.FalseNegatives

		sumPrecision += res.Precision
		sumRecall += res.Recall
		sumF1 += res.F1Score

		// Language breakdown
		lang := res.Language
		if lang == "" {
			lang = "unknown"
		}
		metrics := report.ResultsByLanguage[lang]
		metrics.Cases++
		metrics.TotalDefects += (res.TruePositives + res.FalseNegatives)
		metrics.TruePositives += res.TruePositives
		metrics.FalsePositives += res.FalsePositives
		report.ResultsByLanguage[lang] = metrics
	}

	// Calculate per-language precision, recall, F1
	for lang, metrics := range report.ResultsByLanguage {
		if metrics.TruePositives+metrics.FalsePositives > 0 {
			metrics.Precision = float64(metrics.TruePositives) / float64(metrics.TruePositives+metrics.FalsePositives)
		}
		if metrics.TotalDefects > 0 {
			metrics.Recall = float64(metrics.TruePositives) / float64(metrics.TotalDefects)
		}
		if metrics.Precision+metrics.Recall > 0 {
			metrics.F1Score = 2 * (metrics.Precision * metrics.Recall) / (metrics.Precision + metrics.Recall)
		}
		report.ResultsByLanguage[lang] = metrics
	}

	n := float64(len(results))
	report.TotalGroundTruthDefects = totalTP + totalFN
	report.TotalDetectedFindings = totalTP + totalFP
	report.MacroPrecision = sumPrecision / n
	report.MacroRecall = sumRecall / n
	report.MacroF1Score = sumF1 / n

	return report
}

// LoadBenchmarkCases reads benchmark cases from a JSON dataset file.
func LoadBenchmarkCases(filePath string) ([]BenchmarkCase, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading benchmark cases from %s: %w", filePath, err)
	}

	var cases []BenchmarkCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("failed unmarshaling benchmark cases: %w", err)
	}

	return cases, nil
}
