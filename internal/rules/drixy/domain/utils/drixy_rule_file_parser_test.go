// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rule_file_parser_test.go
// ═══════════════════════════════════════════════════════════════

package utils_test

import (
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/domain/utils"
)

func TestDrixyRuleFileParser(t *testing.T) {
	markdown := `---
title: Avoid SQL String Concatenation
severity: CRITICAL
scope: file
path: "**/*.go"
enabled: true
---

# Avoid SQL Injection

Never format queries with Sprintf or string concatenation. Use parameterized queries.

### Bad Example
` + "```go" + `
query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", id)
` + "```" + `

### Good Example
` + "```go" + `
query := "SELECT * FROM users WHERE id = $1"
rows, err := db.Query(ctx, query, id)
` + "```" + `
`

	t.Run("Parse valid rule file with frontmatter and examples", func(t *testing.T) {
		parsed := utils.ParseDrixyRuleFile(markdown)
		if parsed == nil {
			t.Logf("markdown content:\n%s", markdown)
			t.Fatalf("unexpected nil parsed markdown")
		}

		if parsed.Title != "Avoid SQL String Concatenation" {
			t.Errorf("expected title 'Avoid SQL String Concatenation', got %q", parsed.Title)
		}
		if parsed.Severity != "critical" {
			t.Errorf("expected severity 'critical', got %q", parsed.Severity)
		}
		if parsed.Scope != "file" {
			t.Errorf("expected scope 'file', got %q", parsed.Scope)
		}
		if parsed.Path != "**/*.go" {
			t.Errorf("expected path '**/*.go', got %q", parsed.Path)
		}
		if !parsed.Enabled {
			t.Errorf("expected enabled to be true")
		}

		if len(parsed.Examples) != 2 {
			t.Fatalf("expected 2 examples, got %d", len(parsed.Examples))
		}

		// First example is Bad (IsCorrect == false)
		if parsed.Examples[0].IsCorrect {
			t.Errorf("expected first example to be incorrect (Bad)")
		}

		// Second example is Good (IsCorrect == true)
		if !parsed.Examples[1].IsCorrect {
			t.Errorf("expected second example to be correct (Good)")
		}
	})

	t.Run("IsDrixyRuleTemplateFile path matcher", func(t *testing.T) {
		positivePaths := []string{
			".scandrix/rules/security.md",
			".scandrix/rules/backend/go.md",
			".github/rules/auth.md",
			".gitlab/rules/db.md",
		}
		for _, p := range positivePaths {
			if !utils.IsDrixyRuleTemplateFile(p) {
				t.Errorf("expected path %q to match rule template pattern", p)
			}
		}

		negativePaths := []string{
			"src/rules/evaluator.go",
			"README.md",
			"docs/architecture.md",
			"rules.json",
		}
		for _, p := range negativePaths {
			if utils.IsDrixyRuleTemplateFile(p) {
				t.Errorf("expected path %q to NOT match rule template pattern", p)
			}
		}
	})
}
