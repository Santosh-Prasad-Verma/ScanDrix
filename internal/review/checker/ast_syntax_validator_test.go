package checker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestASTSyntaxValidator_DetectLanguage(t *testing.T) {
	assert.Equal(t, LangGo, DetectLanguage("cmd/main.go", ""))
	assert.Equal(t, LangTypeScript, DetectLanguage("src/app.ts", ""))
	assert.Equal(t, LangJavaScript, DetectLanguage("index.mjs", ""))
	assert.Equal(t, LangTSX, DetectLanguage("components/Button.tsx", ""))
	assert.Equal(t, LangJSX, DetectLanguage("components/Header.jsx", ""))
	assert.Equal(t, LangPython, DetectLanguage("scripts/worker.py", ""))
	assert.Equal(t, LangJSON, DetectLanguage("config.json", ""))
	assert.Equal(t, LangYAML, DetectLanguage(".scandrix.yml", ""))
	assert.Equal(t, LangRust, DetectLanguage("src/lib.rs", ""))
	assert.Equal(t, LangSQL, DetectLanguage("migrations/001_init.sql", ""))
	assert.Equal(t, LangShell, DetectLanguage("deploy.sh", ""))
	assert.Equal(t, LangHTML, DetectLanguage("index.html", ""))
	assert.Equal(t, LangCSS, DetectLanguage("styles/main.css", ""))

	// Explicit override
	assert.Equal(t, LangGo, DetectLanguage("unknown.txt", "go"))
	assert.Equal(t, LangPython, DetectLanguage("temp.dat", "python"))
}

func TestASTSyntaxValidator_GoValidation(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid Full Go File", func(t *testing.T) {
		code := `package main

import "fmt"

func main() {
	fmt.Println("Hello, ScanDrix!")
}
`
		report := v.ValidateFile(ctx, "main.go", code)
		assert.True(t, report.IsValid)
		assert.Equal(t, LangGo, report.Language)
		assert.Empty(t, report.Diagnostics)
		assert.Greater(t, report.Metrics.TokenCount, 0)
	})

	t.Run("Valid Go Snippet", func(t *testing.T) {
		snippet := `mu.Lock()
defer mu.Unlock()
count++`
		report := v.ValidateSnippet(ctx, "worker.go", snippet)
		assert.True(t, report.IsValid)
		assert.Equal(t, LangGo, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Valid Go Declaration Snippet", func(t *testing.T) {
		snippet := `type UserConfig struct {
	Timeout time.Duration
	Retries int
}`
		report := v.ValidateSnippet(ctx, "config.go", snippet)
		assert.True(t, report.IsValid)
		assert.Equal(t, LangGo, report.Language)
	})

	t.Run("Invalid Go Syntax (Unclosed Brace)", func(t *testing.T) {
		code := `package main
func broken() {
	if true {
`
		report := v.ValidateFile(ctx, "broken.go", code)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, SeverityError, report.Diagnostics[0].Severity)
	})

	t.Run("Invalid Go Snippet", func(t *testing.T) {
		snippet := `if true { return 42`
		report := v.ValidateSnippet(ctx, "calc.go", snippet)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxValidator_JSONValidation(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid JSON", func(t *testing.T) {
		code := `{"name": "scandrix", "active": true, "ports": [8080, 8443]}`
		report := v.ValidateFile(ctx, "config.json", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Invalid JSON - Trailing Comma", func(t *testing.T) {
		code := `{"name": "scandrix", "active": true,}`
		report := v.ValidateFile(ctx, "config.json", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, SeverityError, report.Diagnostics[0].Severity)
		assert.Equal(t, "json/syntax", report.Diagnostics[0].RuleID)
	})

	t.Run("Invalid JSON - Unclosed String", func(t *testing.T) {
		code := `{"name": "scandrix`
		report := v.ValidateFile(ctx, "broken.json", code)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxValidator_YAMLValidation(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid YAML with Nested Mappings and Lists", func(t *testing.T) {
		code := `version: "1.0"
scandrix:
  enabled: true
  strictness: high
  rules:
    - id: sec-001
      level: error
`
		report := v.ValidateFile(ctx, ".scandrix.yml", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Invalid YAML - Tab in Indentation", func(t *testing.T) {
		code := "version: 1\n\tenabled: true"
		report := v.ValidateFile(ctx, "config.yaml", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "yaml/no_tabs", report.Diagnostics[0].RuleID)
	})

	t.Run("Invalid YAML - Colon Spacing Missing", func(t *testing.T) {
		badCode := `scandrix:
  key:value
`
		report := v.ValidateFile(ctx, "bad.yaml", badCode)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "yaml/colon_spacing", report.Diagnostics[0].RuleID)
	})

	t.Run("Invalid YAML - Indentation Mismatch", func(t *testing.T) {
		code := `root:
  child:
      grandchild: true
   bad_indent: 123
`
		report := v.ValidateFile(ctx, "bad_indent.yaml", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "yaml/indentation_mismatch", report.Diagnostics[0].RuleID)
	})
}

func TestASTSyntaxValidator_PythonValidation(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid Python Function and Classes", func(t *testing.T) {
		code := `import os

class ReviewManager:
    def __init__(self, workspace_id: str):
        self.workspace_id = workspace_id

    def process(self, findings):
        if not findings:
            return []
        results = [f for f in findings if f.get("severity") == "critical"]
        return results
`
		report := v.ValidateFile(ctx, "worker.py", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Valid Python Triple Quote Multiline", func(t *testing.T) {
		code := `def get_sql():
    return """
    SELECT id, name
    FROM users
    WHERE active = true
    """
`
		report := v.ValidateFile(ctx, "query.py", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Invalid Python - Missing Colon", func(t *testing.T) {
		code := `def calculate_score(val)
    return val * 10
`
		report := v.ValidateFile(ctx, "calc.py", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "python/expected_colon", report.Diagnostics[0].RuleID)
	})

	t.Run("Invalid Python - Indentation Mismatch", func(t *testing.T) {
		code := `def test():
    if True:
        print("yes")
     print("bad indent")
`
		report := v.ValidateFile(ctx, "bad_indent.py", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "python/unindent_mismatch", report.Diagnostics[0].RuleID)
	})

	t.Run("Invalid Python - Tab in Indentation", func(t *testing.T) {
		code := "def test():\n\tpass\n"
		report := v.ValidateFile(ctx, "tab.py", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "python/no_tabs", report.Diagnostics[0].RuleID)
	})
}

func TestASTSyntaxValidator_JavaScriptTypeScriptJSX(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid TypeScript Code", func(t *testing.T) {
		code := `export interface ReviewConfig {
    enabled: boolean;
    threshold: number;
}

export const processReview = (cfg: ReviewConfig): string => {
    const greeting = ` + "`ScanDrix review enabled: ${cfg.enabled}`" + `;
    return greeting;
};
`
		report := v.ValidateFile(ctx, "review.ts", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Valid React TSX Component", func(t *testing.T) {
		code := `import React from 'react';

export const Badge: React.FC<{ label: string }> = ({ label }) => {
    return (
        <div className="badge-container">
            <span className="badge-text">{label}</span>
            <img src="/icons/verified.svg" alt="verified" />
        </div>
    );
};
`
		report := v.ValidateFile(ctx, "Badge.tsx", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Invalid TSX - Unclosed JSX Tag", func(t *testing.T) {
		code := `export const Card = () => {
    return (
        <div className="card">
            <h1>Title</h1>
    );
};
`
		report := v.ValidateFile(ctx, "Card.tsx", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
	})

	t.Run("Invalid TSX - Mismatched Closing JSX Tag", func(t *testing.T) {
		code := `export const Panel = () => {
    return <div><span>Content</div></span>;
};
`
		report := v.ValidateFile(ctx, "Panel.tsx", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "jsx/mismatched_tag", report.Diagnostics[0].RuleID)
	})

	t.Run("Invalid JS - Unmatched Delimiters", func(t *testing.T) {
		code := `const items = [1, 2, 3;`
		report := v.ValidateSnippet(ctx, "test.js", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxValidator_SQLValidation(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid SQL", func(t *testing.T) {
		code := `SELECT id, name, created_at FROM reviews WHERE status = 'ACTIVE' AND severity IN ('critical', 'high');`
		report := v.ValidateFile(ctx, "query.sql", code)
		assert.True(t, report.IsValid)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Invalid SQL - Unmatched Parenthesis", func(t *testing.T) {
		code := `SELECT id FROM reviews WHERE status IN ('ACTIVE', 'PENDING';`
		report := v.ValidateFile(ctx, "bad.sql", code)
		assert.False(t, report.IsValid)
		require.NotEmpty(t, report.Diagnostics)
		assert.Equal(t, "sql/unclosed_paren", report.Diagnostics[0].RuleID)
	})
}

func TestASTSyntaxValidator_CachingAndConcurrency(t *testing.T) {
	v := NewASTSyntaxValidator()
	ctx := context.Background()

	code := `package main
func add(a, b int) int { return a + b }
`
	// First call -> uncached
	r1 := v.ValidateSnippet(ctx, "math.go", code)
	assert.True(t, r1.IsValid)
	assert.False(t, r1.Cached)

	// Second call -> cached
	r2 := v.ValidateSnippet(ctx, "math.go", code)
	assert.True(t, r2.IsValid)
	assert.True(t, r2.Cached)

	// Concurrent invocations under -race
	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			rep := v.ValidateSnippet(ctx, "math.go", code)
			assert.True(t, rep.IsValid)
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent validation timed out")
		}
	}
}
