// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_validation_service_test.go
// ═══════════════════════════════════════════════════════════════

package services_test

import (
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func TestDrixyRulesValidationService(t *testing.T) {
	valSvc := services.NewDrixyRulesValidationService()

	t.Run("ValidateRulesLimit enforcement", func(t *testing.T) {
		// Limited (free tier): <= 10 allowed
		if !valSvc.ValidateRulesLimit(true, 5) {
			t.Errorf("expected 5 rules to pass limit check")
		}
		if !valSvc.ValidateRulesLimit(true, 10) {
			t.Errorf("expected 10 rules to pass limit check")
		}
		if valSvc.ValidateRulesLimit(true, 11) {
			t.Errorf("expected 11 rules to fail limit check")
		}

		// Unlimited (paid tier)
		if !valSvc.ValidateRulesLimit(false, 100) {
			t.Errorf("expected unlimited plan to pass with 100 rules")
		}
	})

	t.Run("FilterDrixyRules separating standard and memory rules", func(t *testing.T) {
		rules := []interfaces.DrixyRule{
			{
				UUID:         "rule-std-1",
				Title:        "Standard Rule 1",
				Rule:         "No hardcoded credentials",
				Type:         interfaces.DrixyRulesTypeStandard,
				Status:       interfaces.DrixyRulesStatusActive,
				RepositoryID: "repo-1",
			},
			{
				UUID:         "rule-mem-1",
				Title:        "Memory Rule 1",
				Rule:         "Team prefers using uuid.NewString() instead of uuid.New().String()",
				Type:         interfaces.DrixyRulesTypeMemory,
				Status:       interfaces.DrixyRulesStatusActive,
				RepositoryID: "repo-1",
			},
			{
				UUID:         "rule-global",
				Title:        "Global Security Rule",
				Rule:         "Always sanitize input",
				Type:         interfaces.DrixyRulesTypeStandard,
				Status:       interfaces.DrixyRulesStatusActive,
				RepositoryID: "global",
			},
			{
				UUID:         "rule-inactive",
				Title:        "Inactive Rule",
				Rule:         "Do not run",
				Type:         interfaces.DrixyRulesTypeStandard,
				Status:       interfaces.DrixyRulesStatusPaused,
				RepositoryID: "repo-1",
			},
		}

		std, mem := valSvc.FilterDrixyRules(rules, "repo-1", "", false)

		if len(std) != 2 {
			t.Fatalf("expected 2 standard rules (repo-1 + global), got %d", len(std))
		}
		if len(mem) != 1 {
			t.Fatalf("expected 1 memory rule, got %d", len(mem))
		}
		if mem[0].UUID != "rule-mem-1" {
			t.Errorf("expected rule-mem-1, got %s", mem[0].UUID)
		}
	})

	t.Run("GetDrixyRulesForFile path matching", func(t *testing.T) {
		rules := []interfaces.DrixyRule{
			{
				UUID:   "rule-go",
				Title:  "Go Rule",
				Rule:   "Check errors",
				Path:   "**/*.go",
				Status: interfaces.DrixyRulesStatusActive,
			},
			{
				UUID:   "rule-ts",
				Title:  "TypeScript Rule",
				Rule:   "Use strict types",
				Path:   "**/*.ts",
				Status: interfaces.DrixyRulesStatusActive,
			},
		}

		goMatches := valSvc.GetDrixyRulesForFile("internal/api/router.go", rules, services.RuleFilterOptions{})
		if len(goMatches) != 1 || goMatches[0].UUID != "rule-go" {
			t.Errorf("expected Go rule to match internal/api/router.go")
		}

		tsMatches := valSvc.GetDrixyRulesForFile("frontend/src/index.ts", rules, services.RuleFilterOptions{})
		if len(tsMatches) != 1 || tsMatches[0].UUID != "rule-ts" {
			t.Errorf("expected TS rule to match frontend/src/index.ts")
		}
	})
}
