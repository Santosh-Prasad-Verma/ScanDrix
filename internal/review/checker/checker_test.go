package checker_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database/warehouse"
	"github.com/scandrix/backend/internal/review/checker"
	"github.com/scandrix/backend/pkg/models"
)

func TestSuggestionCheckWorkerAndAdoptionMetrics(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	store := warehouse.NewEventStore()
	ledger := warehouse.NewFindingsLedger(store)
	worker := checker.NewSuggestionCheckWorker(ledger)

	awsRegex := "AKIA[0-9A-Z]{16}"

	// 1. Finding 1: Developer accepts suggestion EXACTLY
	f1ID := uuid.New()
	_ = ledger.RecordDetection(ctx, models.CodeFinding{
		ID:          f1ID,
		WorkspaceID: wsID,
		Title:       "AWS Key Leak",
		FilePath:    "pkg/auth/key.go",
		Severity:    models.SeverityCritical,
	})

	originalCode1 := `apiKey := "AKIAIOSFODNN7EXAMPLE"`
	suggestedCode1 := `apiKey := os.Getenv("AWS_ACCESS_KEY_ID")`
	committedCode1 := `apiKey := os.Getenv("AWS_ACCESS_KEY_ID")`

	ver1, err := worker.CheckCommit(ctx, f1ID, wsID, 10, originalCode1, suggestedCode1, committedCode1, awsRegex, "sha_commit_1")
	if err != nil || ver1.Status != checker.StatusAcceptedExact {
		t.Fatalf("expected exact acceptance, got %+v, err: %v", ver1, err)
	}
	if ver1.SimilarityScore != 1.0 || ver1.StillVulnerable {
		t.Fatalf("expected similarity 1.0, not vulnerable, got %+v", ver1)
	}

	// Verify ledger marked finding as RESOLVED
	tf1, _ := ledger.GetTrackedFinding(f1ID)
	if tf1.CurrentState != warehouse.StateResolved || tf1.ResolvedInSHA != "sha_commit_1" {
		t.Fatalf("expected ledger state RESOLVED, got %+v", tf1)
	}

	// 2. Finding 2: Developer fixes MANUALLY with their own variable name
	f2ID := uuid.New()
	_ = ledger.RecordDetection(ctx, models.CodeFinding{
		ID:          f2ID,
		WorkspaceID: wsID,
		Title:       "AWS Key Leak 2",
		FilePath:    "pkg/auth/client.go",
		Severity:    models.SeverityCritical,
	})

	originalCode2 := `secretKey := "AKIAIOSFODNN7EXAMPLE"`
	suggestedCode2 := `secretKey := os.Getenv("AWS_SECRET_KEY")`
	committedCode2 := `secretKey := config.GetSecret("AWS_SECRET_KEY")`

	ver2, err := worker.CheckCommit(ctx, f2ID, wsID, 10, originalCode2, suggestedCode2, committedCode2, awsRegex, "sha_commit_2")
	if err != nil || ver2.Status != checker.StatusAcceptedManual {
		t.Fatalf("expected manual acceptance, got %+v, err: %v", ver2, err)
	}
	if ver2.SimilarityScore <= 0 || ver2.StillVulnerable {
		t.Fatalf("expected positive similarity and not vulnerable, got %+v", ver2)
	}

	// 3. Finding 3: Developer didn't fix (still vulnerable)
	f3ID := uuid.New()
	_ = ledger.RecordDetection(ctx, models.CodeFinding{
		ID:          f3ID,
		WorkspaceID: wsID,
		Title:       "AWS Key Leak 3",
		FilePath:    "pkg/auth/test.go",
		Severity:    models.SeverityCritical,
	})

	originalCode3 := `testKey := "AKIAIOSFODNN7EXAMPLE"`
	suggestedCode3 := `testKey := os.Getenv("AWS_TEST_KEY")`
	committedCode3 := `testKey := "AKIAIOSFODNN7EXAMPLE"` // unchanged

	ver3, err := worker.CheckCommit(ctx, f3ID, wsID, 10, originalCode3, suggestedCode3, committedCode3, awsRegex, "sha_commit_3")
	if err != nil || ver3.Status != checker.StatusPending || !ver3.StillVulnerable {
		t.Fatalf("expected pending still-vulnerable status, got %+v", ver3)
	}

	// 4. Compute Adoption Metrics
	metrics := worker.ComputeAdoptionMetrics(wsID)
	if metrics.TotalSuggestions != 3 {
		t.Fatalf("expected 3 total suggestions, got %d", metrics.TotalSuggestions)
	}
	if metrics.AcceptedExactCount != 1 || metrics.AcceptedManualCount != 1 || metrics.PendingCount != 1 {
		t.Fatalf("unexpected counts: %+v", metrics)
	}
	// 2 out of 3 accepted = 0.666...
	if metrics.AdoptionRate < 0.65 || metrics.AdoptionRate > 0.67 {
		t.Fatalf("expected ~0.66 adoption rate, got %f", metrics.AdoptionRate)
	}
}
