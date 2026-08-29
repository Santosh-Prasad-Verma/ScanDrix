package priority_test

import (
	"testing"

	"github.com/scandrix/backend/internal/review/priority"
)

func TestFilePrioritizationScorer(t *testing.T) {
	files := []priority.ScoredFileChange{
		{
			FilePath:  "docs/README.md",
			Status:    priority.StatusModified,
			Additions: 5,
			Deletions: 1,
		},
		{
			FilePath:  "internal/auth/token_manager.go",
			Status:    priority.StatusAdded,
			Additions: 350,
			Deletions: 0,
		},
		{
			FilePath:  "legacy/old_service.go",
			Status:    priority.StatusRemoved,
			Additions: 0,
			Deletions: 500,
		},
	}

	edges := []priority.GraphEdge{
		{Kind: priority.EdgeCalls, SourceFile: "cmd/api/main.go", TargetFile: "internal/auth/token_manager.go"},
		{Kind: priority.EdgeCalls, SourceFile: "internal/api/handler.go", TargetFile: "internal/auth/token_manager.go"},
		{Kind: priority.EdgeInherits, SourceFile: "internal/auth/oauth.go", TargetFile: "internal/auth/token_manager.go"},
	}

	scored := priority.ScoreAndPrioritizeFiles(files, edges)

	if len(scored) != 3 {
		t.Fatalf("expected 3 scored files, got %d", len(scored))
	}

	// 1. Highest priority must be internal/auth/token_manager.go (heavy additions + multiple inbound calls + status added)
	if scored[0].FilePath != "internal/auth/token_manager.go" {
		t.Fatalf("expected token_manager.go to be ranked #1, got %s (score %f)", scored[0].FilePath, scored[0].Score)
	}

	// 2. Lowest priority must be legacy/old_service.go (StatusRemoved multiplier = 0.1)
	if scored[2].FilePath != "legacy/old_service.go" {
		t.Fatalf("expected removed file to be ranked last, got %s (score %f)", scored[2].FilePath, scored[2].Score)
	}

	// Verify inbound calls calculation
	if scored[0].InDegreeCalls != 2 {
		t.Fatalf("expected 2 inbound calls for token_manager.go, got %d", scored[0].InDegreeCalls)
	}
}
