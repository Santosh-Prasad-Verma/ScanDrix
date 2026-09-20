// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ci

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestDetectCI_GitHubActions(t *testing.T) {
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_REPOSITORY", "scandrix/engine")
	os.Setenv("GITHUB_SHA", "abcdef123456")
	os.Setenv("GITHUB_REF", "refs/pull/42/merge")
	defer func() {
		os.Unsetenv("GITHUB_ACTIONS")
		os.Unsetenv("GITHUB_REPOSITORY")
		os.Unsetenv("GITHUB_SHA")
		os.Unsetenv("GITHUB_REF")
	}()

	ctx := DetectCI()
	if !ctx.IsCI || ctx.Provider != ProviderGitHubActions {
		t.Fatalf("expected github_actions, got %+v", ctx)
	}
	if ctx.RepoSlug != "scandrix/engine" || ctx.CommitSHA != "abcdef123456" {
		t.Fatalf("unexpected repo or commit: %+v", ctx)
	}
	if !ctx.IsPR || ctx.PRNumber != 42 {
		t.Fatalf("expected PR 42, got %+v", ctx)
	}
}

func TestGitHubAnnotations(t *testing.T) {
	findings := []models.CodeFinding{
		{
			Title:       "SQL Injection",
			Description: "Unescaped query parameters detected",
			FilePath:    "auth/login.go",
			StartLine:   15,
			EndLine:     18,
			Severity:    models.SeverityCritical,
		},
		{
			Title:       "Variable Shadowing",
			Description: "Variable 'err' shadows outer scope",
			FilePath:    "auth/token.go",
			StartLine:   40,
			EndLine:     40,
			Severity:    models.SeverityLow,
		},
	}

	var buf bytes.Buffer
	EmitGitHubAnnotations(&buf, findings)
	out := buf.String()

	if !strings.Contains(out, "::error file=auth/login.go,line=15,endLine=18,title=[ScanDrix] SQL Injection::Unescaped query parameters detected") {
		t.Fatalf("missing expected error annotation: %s", out)
	}
	if !strings.Contains(out, "::notice file=auth/token.go,line=40,endLine=40,title=[ScanDrix] Variable Shadowing::Variable 'err' shadows outer scope") {
		t.Fatalf("missing expected notice annotation: %s", out)
	}
}

func TestGitLabCodeQuality(t *testing.T) {
	findings := []models.CodeFinding{
		{
			Title:       "Weak Cipher",
			Description: "DES cipher is deprecated",
			FilePath:    "crypto/cipher.go",
			StartLine:   22,
			Severity:    models.SeverityHigh,
		},
	}

	ql := ConvertToGitLabCodeQuality(findings)
	if len(ql) != 1 {
		t.Fatalf("expected 1 GitLab finding, got %d", len(ql))
	}
	if ql[0].Severity != "critical" || ql[0].Location.Path != "crypto/cipher.go" {
		t.Fatalf("unexpected GitLab finding payload: %+v", ql[0])
	}

	data, err := FormatGitLabCodeQualityJSON(findings)
	if err != nil || len(data) == 0 {
		t.Fatalf("failed formatting JSON: %v", err)
	}
}

func TestMarkdownSummary(t *testing.T) {
	ctx := &CIContext{
		Provider:  ProviderGitHubActions,
		IsCI:      true,
		RepoSlug:  "org/repo",
		Branch:    "feature/auth",
		CommitSHA: "1234567890",
		IsPR:      true,
		PRNumber:  101,
	}

	findings := []models.CodeFinding{
		{
			Title:         "Insecure Deserialization",
			Description:   "Untrusted input decoded into struct",
			FilePath:      "api/handler.go",
			StartLine:     55,
			Severity:      models.SeverityHigh,
			SuggestedDiff: "+ // use safe unmarshaler\n",
		},
	}

	md := GenerateMarkdownSummary(ctx, findings, false)
	if !strings.Contains(md, "FAILED") {
		t.Errorf("expected summary to indicate FAILED")
	}
	if !strings.Contains(md, "Insecure Deserialization") {
		t.Errorf("expected summary to mention finding title")
	}
	if !strings.Contains(md, "api/handler.go") {
		t.Errorf("expected summary to mention file path")
	}
}

func TestCIRunner_GateEvaluation(t *testing.T) {
	tempDir := t.TempDir()
	reportFile := filepath.Join(tempDir, "summary.md")

	runner := NewCIRunner()
	findings := []models.CodeFinding{
		{
			Title:     "Info finding",
			FilePath:  "main.go",
			StartLine: 1,
			Severity:  models.SeverityLow,
		},
	}

	var buf bytes.Buffer
	// Gate = critical, finding = low -> PASS
	pass, err := runner.EvaluateFindings(context.Background(), findings, CIOptions{
		FailOnSeverity: "critical",
		ReportPath:     reportFile,
	}, &buf)

	if !pass || err != nil {
		t.Fatalf("expected pass on low finding with fail-on=critical: %v", err)
	}

	// Gate = low, finding = low -> FAIL
	pass2, err2 := runner.EvaluateFindings(context.Background(), findings, CIOptions{
		FailOnSeverity: "low",
	}, &buf)

	if pass2 || err2 == nil {
		t.Fatalf("expected failure when finding severity matches gate")
	}
}
