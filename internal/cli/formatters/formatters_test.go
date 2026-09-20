package formatters

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/types"
)

func mockResult() *types.ReviewResult {
	return &types.ReviewResult{
		ReviewID:      "rev_test_123",
		Status:        "failed",
		Summary:       "1 critical vulnerability and 1 warning detected",
		FilesAnalyzed: 2,
		Issues: []types.ReviewIssue{
			{
				ID:       "iss_1",
				File:     "pkg/auth/token.go",
				Line:     42,
				Column:   10,
				Severity: types.SeverityCritical,
				Category: "security",
				Message:  "Hardcoded JWT secret detected in authentication middleware",
				Suggestion: "Load secret from os.Getenv(\"JWT_SECRET\")",
				RuleID:   "SEC-001",
				Fixable:  true,
				Fix: &types.CodeFix{
					Explanation: "Use environment variable instead of literal",
					OldCode:     "const secret = \"insecure_secret\"",
					NewCode:     "secret := os.Getenv(\"JWT_SECRET\")",
					StartLine:   42,
					EndLine:     42,
				},
				HunkContext: "@@ -40,4 +40,4 @@\n func Auth() {\n- const secret = \"insecure_secret\"\n+ secret := os.Getenv(\"JWT_SECRET\")\n }",
			},
			{
				ID:       "iss_2",
				File:     "pkg/db/query.go",
				Line:     88,
				Severity: types.SeverityWarning,
				Category: "performance",
				Message:  "Unbounded query without LIMIT clause",
				RuleID:   "PERF-004",
				Fixable:  false,
			},
		},
		Stats: types.ReviewStats{
			CriticalCount: 1,
			ErrorCount:    0,
			WarningCount:  1,
			InfoCount:     0,
			FixableCount:  1,
			TotalIssues:   2,
		},
		Duration:   250 * time.Millisecond,
		DurationMs: 250,
		IsBlocking: true,
		ExitCode:   2,
		GitContext: &types.GitInfo{
			Branch:  "feature/auth-hardening",
			HeadSHA: "a1b2c3d4e5f67890",
		},
	}
}

func TestTerminalFormatter(t *testing.T) {
	res := mockResult()
	formatter := NewTerminalFormatter(true, true)

	var buf bytes.Buffer
	err := formatter.Format(&buf, res)
	if err != nil {
		t.Fatalf("TerminalFormatter.Format failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "pkg/auth/token.go:42") {
		t.Errorf("Expected output to contain file location, got: %s", out)
	}
	if !strings.Contains(out, "Hardcoded JWT secret") {
		t.Errorf("Expected output to contain finding message, got: %s", out)
	}
	if !strings.Contains(out, "Critical: 1") {
		t.Errorf("Expected summary breakdown in output, got: %s", out)
	}
}

func TestMarkdownFormatter(t *testing.T) {
	res := mockResult()
	formatter := NewMarkdownFormatter(true)

	var buf bytes.Buffer
	err := formatter.Format(&buf, res)
	if err != nil {
		t.Fatalf("MarkdownFormatter.Format failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "# ScanDrix Code Review Report") {
		t.Errorf("Expected markdown title, got: %s", out)
	}
	if !strings.Contains(out, "| 🔴 Critical | 1 |") {
		t.Errorf("Expected markdown table row, got: %s", out)
	}
	if !strings.Contains(out, "```suggestion") {
		t.Errorf("Expected suggestion block in markdown, got: %s", out)
	}
}

func TestSarifFormatter(t *testing.T) {
	res := mockResult()
	formatter := NewSarifFormatter()

	var buf bytes.Buffer
	err := formatter.Format(&buf, res)
	if err != nil {
		t.Fatalf("SarifFormatter.Format failed: %v", err)
	}

	var parsed SarifLog
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("Failed to parse SARIF JSON output: %v", err)
	}

	if parsed.Version != "2.1.0" {
		t.Errorf("Expected version 2.1.0, got %s", parsed.Version)
	}
	if len(parsed.Runs) != 1 || len(parsed.Runs[0].Results) != 2 {
		t.Errorf("Expected 1 run with 2 results, got %d runs and %d results",
			len(parsed.Runs), len(parsed.Runs[0].Results))
	}
	if parsed.Runs[0].Results[0].RuleID != "SEC-001" {
		t.Errorf("Expected rule SEC-001, got %s", parsed.Runs[0].Results[0].RuleID)
	}
}

func TestJSONFormatter(t *testing.T) {
	res := mockResult()
	formatter := NewJSONFormatter(true)

	var buf bytes.Buffer
	err := formatter.Format(&buf, res)
	if err != nil {
		t.Fatalf("JSONFormatter.Format failed: %v", err)
	}

	var parsed types.ReviewResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if parsed.ReviewID != "rev_test_123" {
		t.Errorf("Expected ReviewID rev_test_123, got %s", parsed.ReviewID)
	}
	if len(parsed.Issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(parsed.Issues))
	}
}
