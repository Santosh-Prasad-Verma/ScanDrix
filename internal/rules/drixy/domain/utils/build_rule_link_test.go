// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: build_rule_link_test.go
// ═══════════════════════════════════════════════════════════════

package utils_test

import (
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/domain/utils"
)

func TestBuildDrixyRuleAppLink(t *testing.T) {
	t.Run("Active rule URL generation", func(t *testing.T) {
		link := utils.BuildDrixyRuleAppLink(utils.BuildDrixyRuleAppLinkParams{
			BaseURL:      "https://app.scandrix.dev",
			RepositoryID: "repo-999",
			RuleID:       "rule-abc",
			TeamID:       "team-123",
			Status:       interfaces.DrixyRulesStatusActive,
			Tab:          utils.DrixyRuleAppLinkTabReviewRules,
		})

		expected := "https://app.scandrix.dev/settings/code-review/repo-999/drixy-rules/rule-abc?tab=review-rules&teamId=team-123"
		if link != expected {
			t.Errorf("expected %s, got %s", expected, link)
		}
	})

	t.Run("Pending rule URL links to parent collection", func(t *testing.T) {
		link := utils.BuildDrixyRuleAppLink(utils.BuildDrixyRuleAppLinkParams{
			BaseURL:      "https://app.scandrix.dev",
			RepositoryID: "repo-999",
			RuleID:       "rule-abc",
			TeamID:       "team-123",
			Status:       interfaces.DrixyRulesStatusPending,
		})

		expected := "https://app.scandrix.dev/settings/code-review/repo-999/drixy-rules?tab=review-rules&teamId=team-123"
		if link != expected {
			t.Errorf("expected %s, got %s", expected, link)
		}
	})

	t.Run("Global scope fallback when repository is empty", func(t *testing.T) {
		link := utils.BuildDrixyRuleAppLink(utils.BuildDrixyRuleAppLinkParams{
			BaseURL: "https://app.scandrix.dev",
			RuleID:  "rule-abc",
			Status:  interfaces.DrixyRulesStatusActive,
		})

		expected := "https://app.scandrix.dev/settings/code-review/global/drixy-rules/rule-abc?tab=review-rules"
		if link != expected {
			t.Errorf("expected %s, got %s", expected, link)
		}
	})
}
