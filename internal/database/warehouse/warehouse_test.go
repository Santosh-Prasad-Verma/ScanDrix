package warehouse_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database/warehouse"
	"github.com/scandrix/backend/pkg/models"
)

func TestEventStoreAndFindingsLedgerLifecycle(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	// 1. Initialize Store, Ledger, and Aggregator
	store := warehouse.NewEventStore()
	ledger := warehouse.NewFindingsLedger(store)
	aggregator := warehouse.NewMetricsAggregator(store, ledger)

	// 2. Append Review Triggered Event
	reviewID := uuid.New()
	reviewEvt, err := warehouse.NewDomainEvent(wsID, reviewID, "REVIEW", warehouse.EventReviewTriggered, map[string]any{
		"repo":   "acme/api",
		"pr_num": 42,
	}, "developer@acme.com")
	if err != nil {
		t.Fatalf("failed to create review domain event: %v", err)
	}

	if err := store.Append(ctx, reviewEvt); err != nil {
		t.Fatalf("failed to append review event: %v", err)
	}

	// 3. Record Code Finding in Ledger
	findingID := uuid.New()
	finding := models.CodeFinding{
		ID:          findingID,
		WorkspaceID: wsID,
		ReviewID:    reviewID,
		FilePath:    "auth/jwt.go",
		StartLine:   45,
		EndLine:     45,
		Severity:    models.SeverityCritical,
		Category:    "SECURITY_SECRET",
		Title:       "Hardcoded Secret",
		Description: "JWT secret hardcoded in code",
		CreatedAt:   time.Now().UTC(),
	}

	if err := ledger.RecordDetection(ctx, finding); err != nil {
		t.Fatalf("failed to record finding detection: %v", err)
	}

	tf, ok := ledger.GetTrackedFinding(findingID)
	if !ok || tf.CurrentState != warehouse.StateDetected {
		t.Fatalf("expected state DETECTED, got %+v", tf)
	}

	// 4. Transition State: Triage Finding
	if err := ledger.TriageFinding(ctx, findingID, "security-lead@acme.com"); err != nil {
		t.Fatalf("failed to triage finding: %v", err)
	}

	tf, _ = ledger.GetTrackedFinding(findingID)
	if tf.CurrentState != warehouse.StateTriaged {
		t.Fatalf("expected state TRIAGED, got %s", tf.CurrentState)
	}

	// 5. Transition State: Dismiss Finding with Reason
	dismissReason := warehouse.ReasonFalsePositive
	if err := ledger.DismissFinding(ctx, findingID, dismissReason, "Verified test mock token", "security-lead@acme.com"); err != nil {
		t.Fatalf("failed to dismiss finding: %v", err)
	}

	tf, _ = ledger.GetTrackedFinding(findingID)
	if tf.CurrentState != warehouse.StateDismissed || *tf.DismissalReason != dismissReason {
		t.Fatalf("expected state DISMISSED, got %+v", tf)
	}

	// 6. Record Second Finding and Resolve it (for MTTR test)
	finding2ID := uuid.New()
	finding2 := models.CodeFinding{
		ID:          finding2ID,
		WorkspaceID: wsID,
		ReviewID:    reviewID,
		FilePath:    "db/query.go",
		StartLine:   22,
		EndLine:     22,
		Severity:    models.SeverityHigh,
		Category:    "SECURITY_INJECTION",
		Title:       "SQL Injection",
		CreatedAt:   time.Now().UTC(),
	}

	if err := ledger.RecordDetection(ctx, finding2); err != nil {
		t.Fatalf("failed to record finding2: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Ensure non-zero time elapsed

	if err := ledger.ResolveFinding(ctx, finding2ID, "sha_commit_fix_12345", "developer@acme.com"); err != nil {
		t.Fatalf("failed to resolve finding2: %v", err)
	}

	tf2, _ := ledger.GetTrackedFinding(finding2ID)
	if tf2.CurrentState != warehouse.StateResolved || tf2.ResolvedInSHA != "sha_commit_fix_12345" {
		t.Fatalf("expected state RESOLVED, got %+v", tf2)
	}

	mttr := ledger.CalculateMTTR(wsID)
	if mttr <= 0 {
		t.Fatalf("expected positive MTTR, got %v", mttr)
	}

	// 7. Test Metrics Aggregator
	start := time.Now().Add(-1 * time.Hour)
	end := time.Now().Add(1 * time.Hour)

	metrics, err := aggregator.Aggregate(ctx, wsID, start, end)
	if err != nil {
		t.Fatalf("failed to aggregate metrics: %v", err)
	}

	if metrics.TotalReviews != 1 {
		t.Fatalf("expected 1 review, got %d", metrics.TotalReviews)
	}
	if metrics.TotalFindings != 2 {
		t.Fatalf("expected 2 findings detected, got %d", metrics.TotalFindings)
	}
	if metrics.DismissedCount != 1 {
		t.Fatalf("expected 1 dismissed, got %d", metrics.DismissedCount)
	}
	if metrics.ResolvedCount != 1 {
		t.Fatalf("expected 1 resolved, got %d", metrics.ResolvedCount)
	}
	if metrics.SeverityCounts[string(models.SeverityCritical)] != 1 {
		t.Fatalf("expected 1 critical finding, got %d", metrics.SeverityCounts[string(models.SeverityCritical)])
	}
}
