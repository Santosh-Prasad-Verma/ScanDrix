package agents_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agents"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
)

func TestAutonomousReviewerDeliberation(t *testing.T) {
	catalog := rules.DefaultCatalog()
	evaluator, err := rules.NewEvaluator(catalog)
	if err != nil {
		t.Fatalf("failed initializing evaluator: %v", err)
	}

	reviewer := agents.NewAutonomousReviewer(evaluator)

	patches := []*diff.FilePatch{
		{
			NewPath:   "auth/tokens.go",
			Additions: 120,
			Deletions: 10,
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 45,
							Content:   `    secretKey := "ghp_1234567890abcdefghijklmnopqrstuvwxyz"`,
						},
					},
				},
			},
		},
		{
			NewPath:   "docs/readme.md",
			Additions: 5,
			Deletions: 2,
		},
	}

	// 1. Test Planning
	plan := reviewer.PlanReview(patches)
	if len(plan.HighRiskFiles) != 1 || plan.HighRiskFiles[0] != "auth/tokens.go" {
		t.Errorf("expected auth/tokens.go to be high risk, got: %+v", plan.HighRiskFiles)
	}
	if plan.EstimatedTokens <= 0 {
		t.Errorf("expected positive token estimate, got %d", plan.EstimatedTokens)
	}

	// 2. Test Execution
	findings, thoughts, err := reviewer.ExecuteAgenticReview(context.Background(), uuid.New(), uuid.New(), patches)
	if err != nil {
		t.Fatalf("ExecuteAgenticReview failed: %v", err)
	}

	if len(thoughts) < 3 {
		t.Fatalf("expected at least 3 deliberation thoughts, got %d", len(thoughts))
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	if findings[0].Title != "Hardcoded GitHub Personal Access Token" {
		t.Errorf("unexpected finding title: %s", findings[0].Title)
	}
}
