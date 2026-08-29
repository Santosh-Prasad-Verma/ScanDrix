package rules

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

func TestEvaluatorDetectsProhibitedCode(t *testing.T) {
	ruleID := uuid.New()
	specs := []RuleSpec{
		{
			ID:          ruleID,
			Name:        "No Raw FMT Debugging",
			PathPattern: "*.go",
			RegexRule:   `fmt\.Println\(`,
			Severity:    models.SeverityLow,
			Category:    "CLEAN_CODE",
			Description: "Avoid fmt.Println in production; use slog instead.",
			Remediation: "Replace with slog.Info()",
		},
	}

	evaluator, err := NewEvaluator(specs)
	if err != nil {
		t.Fatalf("failed to create evaluator: %v", err)
	}

	reviewID := uuid.New()
	workspaceID := uuid.New()

	patches := []*diff.FilePatch{
		{
			NewPath: "cmd/main.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 42,
							Content:   `    fmt.Println("debugging something")`,
						},
						{
							Type:      diff.LineAddition,
							NewLineNo: 43,
							Content:   `    return nil`,
						},
					},
				},
			},
		},
	}

	findings := evaluator.EvaluatePatches(reviewID, workspaceID, patches)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	f := findings[0]
	if f.StartLine != 42 || f.Title != "No Raw FMT Debugging" {
		t.Errorf("unexpected finding attributes: line=%d title=%s", f.StartLine, f.Title)
	}
}
