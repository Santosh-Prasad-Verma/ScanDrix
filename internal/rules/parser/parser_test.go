package parser_test

import (
	"testing"

	"github.com/scandrix/backend/internal/rules/parser"
	"github.com/scandrix/backend/pkg/models"
)

func TestParseMarkdownRule(t *testing.T) {
	doc := `---
title: "No Hardcoded JWT Secrets"
severity: CRITICAL
category: SECURITY_SECRET
paths: ["apps/**", "libs/**", "pkg/**/*.go"]
description: "Ensure JWT secrets are loaded via environment variables."
---

# No Hardcoded JWT Secrets

Always load secrets via environment variables or secret managers.

## Good
` + "```go" + `
jwtKey := os.Getenv("JWT_SECRET")
` + "```" + `

## Bad
` + "```go" + `
jwtKey := "my-super-secret-key-1234"
` + "```" + `

## Remediation
Use internal/config package or environment variable loader.
`

	rule, err := parser.ParseMarkdownRule(".drixy/rules/no-hardcoded-jwt.md", []byte(doc))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if rule.Name != "No Hardcoded JWT Secrets" {
		t.Errorf("expected name 'No Hardcoded JWT Secrets', got '%s'", rule.Name)
	}
	if rule.Severity != models.SeverityCritical {
		t.Errorf("expected severity CRITICAL, got '%s'", rule.Severity)
	}
	if rule.Category != "SECURITY_SECRET" {
		t.Errorf("expected category 'SECURITY_SECRET', got '%s'", rule.Category)
	}
	if len(rule.PathPatterns) != 3 {
		t.Fatalf("expected 3 path patterns, got %d: %+v", len(rule.PathPatterns), rule.PathPatterns)
	}
	if len(rule.GoodExamples) != 1 || len(rule.BadExamples) != 1 {
		t.Errorf("expected 1 good and 1 bad example, got %d good, %d bad", len(rule.GoodExamples), len(rule.BadExamples))
	}
	if rule.Remediation == "" {
		t.Errorf("expected remediation to be populated")
	}

	// Path matching tests
	if !parser.MatchesPath(rule.PathPatterns, "apps/api/main.go") {
		t.Errorf("expected 'apps/api/main.go' to match 'apps/**'")
	}
	if !parser.MatchesPath(rule.PathPatterns, "libs/auth/jwt.ts") {
		t.Errorf("expected 'libs/auth/jwt.ts' to match 'libs/**'")
	}
	if !parser.MatchesPath(rule.PathPatterns, "pkg/crypto/keys.go") {
		t.Errorf("expected 'pkg/crypto/keys.go' to match 'pkg/**/*.go'")
	}
	if parser.MatchesPath(rule.PathPatterns, "docs/README.md") {
		t.Errorf("expected 'docs/README.md' NOT to match path patterns")
	}
}

func TestSplitRulePathGlobs(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{`"apps/**", "libs/**"`, 2},
		{`apps/**;libs/**;pkg/**`, 3},
		{`["app/**", "lib/**"]`, 2},
		{`*`, 1},
		{``, 1},
	}

	for _, c := range cases {
		globs := parser.SplitRulePathGlobs(c.input)
		if len(globs) != c.expected {
			t.Errorf("for input %q: expected %d globs, got %d (%+v)", c.input, c.expected, len(globs), globs)
		}
	}
}
