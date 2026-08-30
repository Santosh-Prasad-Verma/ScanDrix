package catalog_test

import (
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/catalog"
)

func TestAllCatalogRulesValidation(t *testing.T) {
	rules := catalog.GetAllCatalogRules()

	if len(rules) < 100 {
		t.Fatalf("expected at least 100 out-of-the-box rules, got %d", len(rules))
	}

	seenCodes := make(map[string]bool)
	seenIDs := make(map[uuid.UUID]bool)

	for _, r := range rules {
		if r.ID == uuid.Nil {
			t.Errorf("rule %s has nil UUID", r.Code)
		}
		if seenIDs[r.ID] {
			t.Errorf("duplicate rule UUID: %s (%s)", r.ID, r.Code)
		}
		seenIDs[r.ID] = true

		if r.Code == "" {
			t.Error("rule has empty code")
		}
		if seenCodes[r.Code] {
			t.Errorf("duplicate rule code: %s", r.Code)
		}
		seenCodes[r.Code] = true

		if r.Name == "" {
			t.Errorf("rule %s has empty name", r.Code)
		}
		if r.Category == "" {
			t.Errorf("rule %s has empty category", r.Code)
		}
		if r.Description == "" {
			t.Errorf("rule %s has empty description", r.Code)
		}
		if r.Remediation == "" {
			t.Errorf("rule %s has empty remediation", r.Code)
		}
		if r.RegexRule == "" {
			t.Errorf("rule %s has empty regex rule", r.Code)
		}

		// Verify that all 100+ regular expressions compile without error
		_, err := regexp.Compile(r.RegexRule)
		if err != nil {
			t.Errorf("rule %s regex failed compilation: %v (pattern: %s)", r.Code, err, r.RegexRule)
		}
	}
}

func TestFilterByLanguage(t *testing.T) {
	goRules := catalog.GetRulesByLanguage("GO")
	if len(goRules) == 0 {
		t.Error("expected Go rules")
	}

	pyRules := catalog.GetRulesByLanguage("PYTHON")
	if len(pyRules) == 0 {
		t.Error("expected Python rules")
	}

	tsRules := catalog.GetRulesByLanguage("TYPESCRIPT")
	if len(tsRules) == 0 {
		t.Error("expected TypeScript rules")
	}

	dockerRules := catalog.GetRulesByLanguage("DOCKER")
	if len(dockerRules) == 0 {
		t.Error("expected Docker rules")
	}
}
