package rules_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestHierarchyResolver(t *testing.T) {
	resolver := rules.NewHierarchyResolver()

	global := []rules.RuleSpec{
		{ID: uuid.New(), Name: "No Hardcoded Secrets", Severity: models.SeverityCritical},
		{ID: uuid.New(), Name: "Prefer Const", Severity: models.SeverityLow},
	}

	org := []rules.RuleSpec{
		{ID: uuid.New(), Name: "Require RLS", Severity: models.SeverityHigh},
	}

	team := []rules.RuleSpec{
		{ID: uuid.New(), Name: "Prefer Const", Severity: models.SeverityMedium}, // Overrides global
	}

	repo := []rules.RuleSpec{
		{ID: uuid.New(), Name: "Custom API Rule", Severity: models.SeverityHigh},
	}

	disabled := []string{"Require RLS"}
	overrides := map[string]models.FindingSeverity{
		"prefer const": models.SeverityHigh,
	}

	effective := resolver.ResolveActiveRules(global, org, team, repo, disabled, overrides)

	if len(effective) != 3 {
		t.Fatalf("expected 3 active rules after disable, got %d", len(effective))
	}

	for _, r := range effective {
		if r.Name == "Require RLS" {
			t.Errorf("expected Require RLS to be disabled")
		}
		if r.Name == "Prefer Const" && r.Severity != models.SeverityHigh {
			t.Errorf("expected Prefer Const severity override to HIGH, got %s", r.Severity)
		}
	}
}
