
package ui

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestRenderIssueDetailsLines(t *testing.T) {
	finding := models.CodeFinding{
		FilePath:      "cmd/main.go",
		StartLine:     42,
		Severity:      models.SeverityHigh,
		Category:      "Security",
		Title:         "SEC-001 Potential hardcoded token",
		Description:   "Potential hardcoded token",
		Remediation:   "Use environment variable instead",
		SuggestedDiff: "- token := \"secret\"\n+ token := os.Getenv(\"SECRET\")",
	}

	lines := RenderIssueDetailsLines(finding)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "cmd/main.go") {
		t.Fatalf("expected file in output, got %s", joined)
	}
	if !strings.Contains(joined, "42") {
		t.Fatalf("expected line in output, got %s", joined)
	}
	if !strings.Contains(joined, "SEC-001") {
		t.Fatalf("expected title in output, got %s", joined)
	}
	if !strings.Contains(joined, "Suggested diff available") {
		t.Fatalf("expected suggested diff indicator, got %s", joined)
	}
}

func TestRenderFixPreviewLines(t *testing.T) {
	findingWithFix := models.CodeFinding{
		SuggestedDiff: "- token := \"secret\"\n+ token := os.Getenv(\"SECRET\")",
	}

	lines := RenderFixPreviewLines(findingWithFix)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "- token := \"secret\"") || !strings.Contains(joined, "+ token := os.Getenv(\"SECRET\")") {
		t.Fatalf("expected diff preview, got %s", joined)
	}

	findingNoFix := models.CodeFinding{}
	noFixLines := RenderFixPreviewLines(findingNoFix)
	if !strings.Contains(strings.Join(noFixLines, "\n"), "No suggested diff available") {
		t.Fatalf("expected no fix message, got %v", noFixLines)
	}
}

func TestRenderFileHeaderAndSummaryLines(t *testing.T) {
	headerLines := RenderFileHeaderLines("pkg/auth/auth.go", 3)
	headerJoined := strings.Join(headerLines, "\n")
	if !strings.Contains(headerJoined, "pkg/auth/auth.go") || !strings.Contains(headerJoined, "3 issues in this file") {
		t.Fatalf("unexpected header output: %s", headerJoined)
	}

	summaryLines := RenderReviewSummaryLines(10, 4)
	summaryJoined := strings.Join(summaryLines, "\n")
	if !strings.Contains(summaryJoined, "Total issues: ") || !strings.Contains(summaryJoined, "Fixed: ") || !strings.Contains(summaryJoined, "Remaining: ") {
		t.Fatalf("unexpected summary output: %s", summaryJoined)
	}
}
