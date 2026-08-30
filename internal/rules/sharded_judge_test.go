package rules_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestShardedRuleJudge(t *testing.T) {
	ctx := context.Background()

	// 12 rules -> with shard size 4 = 3 shards
	var testRules []rules.RuleSpec
	for i := 0; i < 12; i++ {
		testRules = append(testRules, rules.RuleSpec{
			ID:       uuid.New(),
			Name:     "Test Rule",
			Severity: models.SeverityHigh,
		})
	}

	patches := []*diff.FilePatch{
		{
			NewPath: "src/auth.go",
			Hunks: []diff.Hunk{
				{NewStart: 10, NewLines: 5},
			},
		},
	}

	mockExecutor := func(ctx context.Context, shardIndex int, rList []rules.RuleSpec, pList []*diff.FilePatch) ([]models.CodeFinding, error) {
		if shardIndex == 1 {
			// Simulate partial failure on shard 1
			return nil, errors.New("provider timeout on shard 1")
		}
		var findings []models.CodeFinding
		for _, r := range rList {
			findings = append(findings, models.CodeFinding{
				ID:       uuid.New(),
				Title:    r.Name,
				Severity: r.Severity,
			})
		}
		return findings, nil
	}

	judge := rules.NewShardedRuleJudge(4, 2, mockExecutor)
	findings, stats := judge.EvaluateSharded(ctx, uuid.New(), uuid.New(), testRules, patches)

	if stats.ShardsRun != 3 {
		t.Fatalf("expected 3 shards run, got %d", stats.ShardsRun)
	}
	if stats.ShardsSucceeded != 2 {
		t.Fatalf("expected 2 shards succeeded, got %d", stats.ShardsSucceeded)
	}
	if stats.ShardsErrored != 1 {
		t.Fatalf("expected 1 shard errored, got %d", stats.ShardsErrored)
	}
	if len(findings) != 8 {
		t.Fatalf("expected 8 findings from successful shards (4+4), got %d", len(findings))
	}
	if stats.Duration <= 0 {
		t.Errorf("expected positive duration")
	}
}
