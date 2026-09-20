// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Unit Tests
// File: drixy_rule_detector_compiler_test.go
// ═══════════════════════════════════════════════════════════════

package services_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func TestDrixyRuleDetectorCompiler_ExampleGating(t *testing.T) {
	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(ruleLikeRepo)
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, ruleLikeSvc)

	compiler := services.NewDrixyRuleDetectorCompiler(rulesSvc, nil)

	ctx := context.Background()

	// 1. Rule with detector pattern already provided and correct examples
	ruleWithExamples := &interfaces.DrixyRule{
		UUID:   "rule-detector-1",
		Title:  "Disallow console.log",
		Rule:   "Do not use console.log statements in production code",
		Status: interfaces.DrixyRulesStatusActive,
		Detector: &interfaces.DrixyRuleDetector{
			Type:    "regex",
			Pattern: `console\.log\(`,
		},
		Examples: []interfaces.DrixyRulesExample{
			{Snippet: "console.log('debug');", IsCorrect: false},
			{Snippet: "logger.Info('info');", IsCorrect: true},
		},
	}

	res, err := compiler.CompileAndSave(ctx, "org-test", "team-test", ruleWithExamples.UUID, ruleWithExamples)
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	if !res.Compiled {
		t.Fatalf("expected rule to be compiled, got decline reason: %s", res.DeclineReason)
	}

	// 2. Test sweep matching against added diff lines
	sweepService := services.NewDrixyRuleDetectorSweepService()
	addedLines := []string{
		"logger.Info(\"running\");",
		"console.log(\"bad log statement\");",
	}
	matches := sweepService.SweepDiffAddedLines(ctx, []interfaces.DrixyRule{*ruleWithExamples}, "src/index.js", addedLines)

	if len(matches) != 1 {
		t.Fatalf("expected 1 match from detector sweep, got %d", len(matches))
	}

	findings := sweepService.ConvertToFindings(matches, [16]byte{})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if string(findings[0].Severity) != "HIGH" {
		t.Fatalf("expected HIGH severity, got %s", findings[0].Severity)
	}
}

