package rules

import (
	"testing"
)

func TestParseRuleFile(t *testing.T) {
	ruleMarkdown := `---
title: Ensure Safe Goroutine Concurrency
severity_min: critical
scope: file
path:
  - "**/*.go"
  - "!vendor/**"
enabled: true
uuid: 123e4567-e89b-12d3-a456-426614174000
---

Always pass context.Context to background goroutines and handle cancellations.

### Bad Example
` + "```go" + `
go func() {
    doWork()
}()
` + "```" + `

### Good Example
` + "```go" + `
go func(ctx context.Context) {
    select {
    case <-ctx.Done():
        return
    default:
        doWork()
    }
}(ctx)
` + "```"

	parsed := ParseRuleFile(ruleMarkdown)
	if parsed == nil {
		t.Fatalf("expected non-nil parsed rule")
	}

	if parsed.Title != "Ensure Safe Goroutine Concurrency" {
		t.Fatalf("unexpected title: %q", parsed.Title)
	}

	if parsed.Severity != "critical" {
		t.Fatalf("unexpected severity: %q", parsed.Severity)
	}

	if parsed.Scope != "file" {
		t.Fatalf("unexpected scope: %q", parsed.Scope)
	}

	if len(parsed.Examples) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(parsed.Examples))
	}

	if parsed.Examples[0].IsCorrect != false {
		t.Fatalf("expected first example to be bad")
	}

	if parsed.Examples[1].IsCorrect != true {
		t.Fatalf("expected second example to be good")
	}
}

func TestPatterns(t *testing.T) {
	ruleGlobs := "src/**/*.ts, internal/**/*.go, !vendor/**"

	if !MatchFileAgainstRule("internal/common/crypto/crypto.go", ruleGlobs) {
		t.Fatalf("expected internal/common/crypto/crypto.go to match")
	}

	if MatchFileAgainstRule("vendor/github.com/pkg/errors.go", ruleGlobs) {
		t.Fatalf("expected vendor path to be excluded by negative glob")
	}

	if !MatchFileAgainstRule("src/api/routes.ts", ruleGlobs) {
		t.Fatalf("expected src/api/routes.ts to match")
	}

	if MatchFileAgainstRule("docs/README.md", ruleGlobs) {
		t.Fatalf("expected docs/README.md to not match")
	}
}

func TestIsRuleTemplateFile(t *testing.T) {
	if !IsRuleTemplateFile(".scandrix/rules/security.md") {
		t.Fatalf("expected .scandrix/rules/security.md to be recognized")
	}

	if !IsRuleTemplateFile("rules/concurrency.md") {
		t.Fatalf("expected rules/concurrency.md to be recognized")
	}

	if IsRuleTemplateFile("README.md") {
		t.Fatalf("expected README.md not to be recognized as rule template")
	}
}

func TestFilePatterns_AgentsRulesDiscovery(t *testing.T) {
	t.Run("ships the .agents/rules/** pattern in RuleFilePatterns", func(t *testing.T) {
		found := false
		for _, p := range RuleFilePatterns {
			if p == ".agents/rules/**" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected .agents/rules/** in RuleFilePatterns")
		}
	})

	t.Run("recognises repo-root .agents/rules files as IDE rule sources", func(t *testing.T) {
		if !IsIDERuleSource(".agents/rules/architecture.md") {
			t.Fatal("expected .agents/rules/architecture.md to be recognized as IDE rule source")
		}
	})

	t.Run("recognises nested .agents/rules files (monorepo subdir)", func(t *testing.T) {
		if !IsIDERuleSource("applications/sales/.agents/rules/style.md") {
			t.Fatal("expected applications/sales/.agents/rules/style.md to be recognized as IDE rule source")
		}
	})

	t.Run("scopes a nested .agents/rules source to its repo subdir", func(t *testing.T) {
		subdir := ExtractRepoSubdirFromIDESource("applications/sales/.agents/rules/style.md")
		if subdir == nil || *subdir != "applications/sales" {
			t.Fatalf("expected 'applications/sales', got %v", subdir)
		}
	})

	t.Run("treats a repo-root .agents/rules file as repo-wide", func(t *testing.T) {
		subdir := ExtractRepoSubdirFromIDESource(".agents/rules/style.md")
		if subdir != nil {
			t.Fatalf("expected nil for repo-root .agents/rules, got %v", *subdir)
		}
	})
}

