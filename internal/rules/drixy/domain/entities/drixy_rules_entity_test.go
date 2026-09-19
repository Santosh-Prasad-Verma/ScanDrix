// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Unit Tests
// File: drixy_rules_entity_test.go
// ═══════════════════════════════════════════════════════════════

package entities_test

import (
	"testing"
	"time"

	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

func TestDrixyRulesEntity_BasicOperations(t *testing.T) {
	now := time.Now().UTC()
	entity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		OrganizationID: "org-123",
		Rules: []interfaces.DrixyRule{
			{
				UUID:         "rule-1",
				Title:        "No raw SQL",
				Rule:         "Never concatenate user input into SQL queries",
				Status:       interfaces.DrixyRulesStatusActive,
				RepositoryID: "repo-abc",
				CreatedAt:    &now,
			},
			{
				UUID:         "rule-2",
				Title:        "Paused rule",
				Rule:         "Temporary paused check",
				Status:       interfaces.DrixyRulesStatusPaused,
				RepositoryID: "repo-abc",
				CreatedAt:    &now,
			},
		},
	})

	if entity.OrganizationID() != "org-123" {
		t.Fatalf("expected org-123, got %s", entity.OrganizationID())
	}

	if entity.CountActiveRules() != 1 {
		t.Fatalf("expected 1 active rule, got %d", entity.CountActiveRules())
	}

	// Add new rule
	newRule := interfaces.DrixyRule{
		UUID:         "rule-3",
		Title:        "Always sanitize HTML",
		Rule:         "Escape HTML output",
		Status:       interfaces.DrixyRulesStatusActive,
		RepositoryID: "repo-abc",
	}
	entity.AddOrUpdateRule(newRule)

	if entity.CountActiveRules() != 2 {
		t.Fatalf("expected 2 active rules, got %d", entity.CountActiveRules())
	}

	// Delete rule
	deleted := entity.DeleteRule("rule-1")
	if !deleted {
		t.Fatalf("expected rule-1 to be deleted")
	}

	if entity.CountActiveRules() != 1 {
		t.Fatalf("expected 1 active rule after deletion, got %d", entity.CountActiveRules())
	}
}

func TestDrixyRulesEntity_SeverityResolution(t *testing.T) {
	rule := &interfaces.DrixyRule{
		Severity: "CRITICAL",
	}
	sev := interfaces.ResolveDrixyRuleSeverityLevel(rule)
	if string(sev) != "CRITICAL" {
		t.Fatalf("expected CRITICAL severity, got %s", sev)
	}

	ruleLow := &interfaces.DrixyRule{
		Severity: "low",
	}
	sevLow := interfaces.ResolveDrixyRuleSeverityLevel(ruleLow)
	if string(sevLow) != "LOW" {
		t.Fatalf("expected LOW severity, got %s", sevLow)
	}
}
