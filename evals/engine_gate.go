// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package evals

import (
	"context"
	"fmt"
	"os"

	"github.com/scandrix/backend/evals/anchoring"
	"github.com/scandrix/backend/evals/dedup"
	"github.com/scandrix/backend/evals/drixyrules"
	"github.com/scandrix/backend/evals/format"
	"github.com/scandrix/backend/evals/investigation"
	"github.com/scandrix/backend/evals/parser"
	"github.com/scandrix/backend/evals/prsummary"
	"github.com/scandrix/backend/evals/promotion"
	"github.com/scandrix/backend/evals/scorer"
	"github.com/scandrix/backend/evals/secondary"
	"github.com/scandrix/backend/evals/severity"
	"github.com/scandrix/backend/evals/structuredoutputs"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/review/agentcore"
)

// EvalProfile defines the evaluation depth and execution criteria.
type EvalProfile string

const (
	ProfileHarness EvalProfile = "harness" // offline validation of contracts & schemas
	ProfileLocal   EvalProfile = "local"   // fast developer gate
	ProfileCI      EvalProfile = "ci"      // full regression gate
)

// PreflightReport summarizes the readiness of the evaluation environment.
type PreflightReport struct {
	Profile       EvalProfile `json:"profile"`
	Passed        bool        `json:"passed"`
	Checks        []string    `json:"checks"`
	Warnings      []string    `json:"warnings,omitempty"`
	FatalFailures []string    `json:"fatal_failures,omitempty"`
}

// RunPreflight validates that all 11 core evaluation domain engines are sound.
func RunPreflight(profile EvalProfile) PreflightReport {
	report := PreflightReport{
		Profile: profile,
		Passed:  true,
	}

	// 1. Validate anchoring evaluator
	hunk := anchoring.DiffHunk{FilePath: "main.go", NewStart: 10, NewLength: 20}
	anchorRes := anchoring.EvaluateAnchor(anchoring.CommentAnchor{FilePath: "main.go", LineStart: 15}, []anchoring.DiffHunk{hunk})
	if !anchorRes.Valid {
		report.FatalFailures = append(report.FatalFailures, "anchoring evaluator failed sanity check")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "anchoring evaluator verified")
	}

	// 2. Validate dedup engine
	dedupRes := dedup.JaccardSimilarity("null pointer exception in handler", "null pointer exception in handler")
	if dedupRes < 0.99 {
		report.FatalFailures = append(report.FatalFailures, "dedup engine failed similarity check")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "dedup similarity engine verified")
	}

	// 3. Validate severity classifier
	sevRes := severity.EvaluateSeverity("critical", "critical")
	if !sevRes.IsCorrect {
		report.FatalFailures = append(report.FatalFailures, "severity evaluator failed exact match check")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "severity evaluator verified")
	}

	// 4. Validate structured format engine
	formatRes := format.EvaluateStructuredFormat(`{"valid": true}`)
	if !formatRes.ValidJSON {
		report.FatalFailures = append(report.FatalFailures, "format evaluator failed direct parse check")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "format parser verified")
	}

	// 5. Validate investigation evaluator
	invRes := investigation.EvaluateInvestigation(nil, nil, 0)
	if !invRes.Passed {
		report.FatalFailures = append(report.FatalFailures, "investigation evaluator failed sanity check")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "investigation evaluator verified")
	}

	// 6. Validate drixy-rules behavioral scorer
	sites := []drixyrules.Site{{File: "auth.go", Line: 10}}
	flags := []drixyrules.Site{{File: "auth.go", Line: 11}} // within tolerance 2
	caseScore := drixyrules.ScoreCase(sites, flags, 2)
	if caseScore.Caught != 1 {
		report.FatalFailures = append(report.FatalFailures, "drixyrules behavioral scorer failed tolerance match")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "drixyrules behavioral scorer verified")
	}

	// 7. Validate diff fixture parser
	rawDiff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,5 +1,6 @@ package main\n+func test() {}"
	passedParser, _ := parser.EvaluateParser(rawDiff, 1, 1)
	if !passedParser {
		report.FatalFailures = append(report.FatalFailures, "diff parser evaluator failed sample parse")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "diff fixture parser verified")
	}

	// 8. Validate PR summary evaluator
	sumRes := prsummary.EvaluatePRSummary("# Overview\nWalkthrough of changed files:\n- main.go\nRisk assessment: low impact.")
	if !sumRes.Passed {
		report.FatalFailures = append(report.FatalFailures, "PR summary evaluator failed structured sample")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "PR summary evaluator verified")
	}

	// 9. Validate promotion evaluator
	finding := agentcore.FinderSuggestion{
		RelevantFile:      "auth.go",
		ExistingCode:      "password := \"123\"",
		ImprovedCode:      "password := os.Getenv(\"PASS\")",
		Confidence:        0.95,
		SuggestionContent: "Hardcoded secret",
	}
	promDec := promotion.EvaluatePromotion(finding)
	if !promDec.Promoted {
		report.FatalFailures = append(report.FatalFailures, "promotion evaluator failed valid candidate")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "promotion evaluator verified")
	}

	// 10. Validate multi-pillar scorer
	compScore := scorer.ComputeCompositeScore(0.9, 0.85, 1.0, 0.95)
	if !compScore.Passed {
		report.FatalFailures = append(report.FatalFailures, "composite quality scorer failed high-scoring input")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "composite quality scorer verified")
	}

	// 11. Validate secondary passes
	secCases := secondary.BuiltInSecondaryTestCases()
	secFinding := agentcore.FinderSuggestion{
		RelevantFile:      "merge.go",
		SuggestionContent: "Prototype pollution risk via unsafe Object.assign or constructor lookup",
		ImprovedCode:      "Validate key not in ['__proto__', 'constructor']",
	}
	secMatched := secondary.EvaluateSecondaryPass([]agentcore.FinderSuggestion{secFinding}, secCases[0])
	if !secMatched {
		report.FatalFailures = append(report.FatalFailures, "secondary vulnerability evaluator failed prototype pollution check")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "secondary vulnerability evaluator verified")
	}

	// 12. Validate structured output extractor
	type samplePayload struct {
		Ready bool `json:"ready"`
	}
	extracted, err := structuredoutputs.EvaluateStructuredOutputExtraction[samplePayload]("```json\n{\"ready\": true}\n```")
	if err != nil || !extracted.Ready {
		report.FatalFailures = append(report.FatalFailures, "structured output extractor failed markdown unwrap")
		report.Passed = false
	} else {
		report.Checks = append(report.Checks, "structured output extractor verified")
	}

	if profile == ProfileCI {
		if os.Getenv("DATABASE_URL") == "" {
			report.Warnings = append(report.Warnings, "DATABASE_URL unset in CI profile")
		}
	}

	return report
}

// ExecuteEngineGate runs the evaluation gate preflight and logs summary.
func ExecuteEngineGate(ctx context.Context, profile EvalProfile) error {
	report := RunPreflight(profile)

	fmt.Printf("════ ScanDrix AI Complete 11-Domain Eval Preflight · Profile=%s ════\n", report.Profile)
	for _, c := range report.Checks {
		fmt.Printf("  OK    %s\n", c)
	}
	for _, w := range report.Warnings {
		fmt.Printf("  WARN  %s\n", w)
	}
	for _, f := range report.FatalFailures {
		fmt.Printf("  FAIL  %s\n", f)
	}

	if !report.Passed {
		return fmt.Errorf("eval preflight failed with %d fatal failures", len(report.FatalFailures))
	}

	fmt.Println("\nEval preflight passed successfully across all 11 domains.")
	return nil
}

// RunHarnessGate executes the default harness profile gate.
func RunHarnessGate(ctx context.Context) error {
	return ExecuteEngineGate(ctx, ProfileHarness)
}

// Dummy reference to keep contracts imported
var _ = contracts.JSONSchema{}
