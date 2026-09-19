// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package secondary

import (
	"strings"

	"github.com/scandrix/backend/internal/review/agentcore"
)

// VulnerabilityClass categorizes critical secondary vulnerability patterns.
type VulnerabilityClass string

const (
	ClassPrototypePollution VulnerabilityClass = "prototype_pollution"
	ClassDenialOfService    VulnerabilityClass = "denial_of_service"
	ClassAsyncMisuse        VulnerabilityClass = "async_misuse"
	ClassArgumentOrder      VulnerabilityClass = "argument_order"
)

// SecondaryEvalCase defines a known vulnerability scenario.
type SecondaryEvalCase struct {
	ID          string             `json:"id"`
	Class       VulnerabilityClass `json:"class"`
	Description string             `json:"description"`
	TargetFile  string             `json:"target_file"`
	Keywords    []string           `json:"keywords"`
}

// EvaluateSecondaryPass checks if candidate findings successfully caught the target secondary vulnerability.
func EvaluateSecondaryPass(findings []agentcore.FinderSuggestion, testCase SecondaryEvalCase) bool {
	for _, f := range findings {
		if testCase.TargetFile != "" && !strings.Contains(strings.ToLower(f.RelevantFile), strings.ToLower(testCase.TargetFile)) {
			continue
		}

		content := strings.ToLower(f.SuggestionContent + " " + f.OneSentenceSummary + " " + f.ImprovedCode)
		matched := true
		for _, kw := range testCase.Keywords {
			if !strings.Contains(content, strings.ToLower(kw)) {
				matched = false
				break
			}
		}

		if matched {
			return true
		}
	}

	return false
}

// BuiltInSecondaryTestCases returns the gold benchmark secondary test scenarios.
func BuiltInSecondaryTestCases() []SecondaryEvalCase {
	return []SecondaryEvalCase{
		{
			ID:          "sec-proto-pollute-01",
			Class:       ClassPrototypePollution,
			Description: "Unsafe object merge allowing __proto__ or constructor modification",
			TargetFile:  "merge",
			Keywords:    []string{"proto", "pollution"},
		},
		{
			ID:          "sec-cache-dos-01",
			Class:       ClassDenialOfService,
			Description: "Unbounded cache key growth causing OOM denial of service",
			TargetFile:  "cache",
			Keywords:    []string{"unbounded", "eviction"},
		},
		{
			ID:          "sec-async-foreach-01",
			Class:       ClassAsyncMisuse,
			Description: "Async callback inside array forEach resulting in unhandled concurrency",
			TargetFile:  "worker",
			Keywords:    []string{"async", "foreach"},
		},
	}
}
