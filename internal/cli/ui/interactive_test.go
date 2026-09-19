// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/pkg/models"
)

func sampleFindings() []models.CodeFinding {
	return []models.CodeFinding{
		{
			ID:          uuid.New(),
			FilePath:    "pkg/auth/token.go",
			StartLine:   42,
			EndLine:     45,
			Severity:    models.FindingSeverity("HIGH"),
			Category:    "security",
			Title:       "Missing Token Expiration Validation",
			Description: "JWT token validation does not verify the exp claim.",
			Remediation: "Ensure claims.VerifyExpiresAt(time.Now(), true) is called.",
		},
		{
			ID:            uuid.New(),
			FilePath:      "pkg/auth/token.go",
			StartLine:     60,
			EndLine:       62,
			Severity:      models.FindingSeverity("MEDIUM"),
			Category:      "code_quality",
			Title:         "Error Swallowed in Decode",
			Description:   "Error returned by json.Unmarshal is ignored.",
			Remediation:   "Return err instead of nil.",
			SuggestedDiff: "+ return err",
		},
		{
			ID:          uuid.New(),
			FilePath:    "internal/db/conn.go",
			StartLine:   15,
			EndLine:     18,
			Severity:    models.FindingSeverity("CRITICAL"),
			Category:    "security",
			Title:       "SQL Injection Vulnerability",
			Description: "Query is formatted using fmt.Sprintf instead of prepared statement.",
			Remediation: "Use db.QueryRowContext with ? placeholders.",
		},
	}
}

func TestGroupFindingsByFile(t *testing.T) {
	findings := sampleFindings()
	grouped := GroupFindingsByFile(findings)

	if len(grouped) != 2 {
		t.Fatalf("expected 2 files, got %d", len(grouped))
	}
	if len(grouped["pkg/auth/token.go"]) != 2 {
		t.Errorf("expected 2 findings in token.go, got %d", len(grouped["pkg/auth/token.go"]))
	}
	if len(grouped["internal/db/conn.go"]) != 1 {
		t.Errorf("expected 1 finding in conn.go, got %d", len(grouped["internal/db/conn.go"]))
	}
}

func TestGetFileStats(t *testing.T) {
	findings := sampleFindings()
	stats := GetFileStats(findings)

	if stats.Critical != 1 {
		t.Errorf("expected 1 critical, got %d", stats.Critical)
	}
	if stats.High != 1 {
		t.Errorf("expected 1 high, got %d", stats.High)
	}
	if stats.Medium != 1 {
		t.Errorf("expected 1 medium, got %d", stats.Medium)
	}
	if stats.Low != 0 {
		t.Errorf("expected 0 low, got %d", stats.Low)
	}
}

func TestGenerateFixPrompt(t *testing.T) {
	findings := sampleFindings()
	prompt := GenerateFixPrompt("pkg/auth/token.go", findings[:2])

	if !strings.Contains(prompt, "Fix the following issues in pkg/auth/token.go:") {
		t.Error("prompt missing file header")
	}
	if !strings.Contains(prompt, "Missing Token Expiration Validation") {
		t.Error("prompt missing finding title")
	}
	if !strings.Contains(prompt, "Suggested Diff:") {
		t.Error("prompt missing suggested diff")
	}
}

func TestGenerateFixPromptAll(t *testing.T) {
	findings := sampleFindings()
	grouped := GroupFindingsByFile(findings)
	prompt := GenerateFixPromptAll(grouped)

	if !strings.Contains(prompt, "Fix the following 3 issues across 2 files") {
		t.Errorf("unexpected header in prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "## File 1/2:") || !strings.Contains(prompt, "## File 2/2:") {
		t.Error("prompt missing file section headers")
	}
}

func TestGetFixableFindings(t *testing.T) {
	findings := sampleFindings()
	fixable := GetFixableFindings(findings)

	if len(fixable) != 1 {
		t.Fatalf("expected 1 fixable finding, got %d", len(fixable))
	}
	if fixable[0].Title != "Error Swallowed in Decode" {
		t.Errorf("unexpected fixable finding: %s", fixable[0].Title)
	}
}

func TestInteractiveUI_RunEmpty(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("q\n")
	ui := NewInteractiveUI(input, &buf)

	res := &review.ReviewResult{
		FilesAnalyzed: 5,
		Findings:      []models.CodeFinding{},
	}

	err := ui.Run(res)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected clean message, got %s", out)
	}
}

func TestInteractiveUI_RunQuit(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("q\n")
	ui := NewInteractiveUI(input, &buf)

	res := &review.ReviewResult{
		FilesAnalyzed: 2,
		Findings:      sampleFindings(),
	}

	err := ui.Run(res)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Select a file to inspect") {
		t.Errorf("expected menu display, got %s", out)
	}
}
