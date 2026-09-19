package lease_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/sandbox/lease"
)

func TestMemoryLeaseRepository_Lifecycle(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()

	orgID := uuid.New().String()
	prKey := orgID + ":backend:42"

	// 1. Initial Acquire (Creator Path)
	doc1, err := repo.UpsertAcquire(ctx, prKey, 30*time.Minute, "review")
	if err != nil {
		t.Fatalf("UpsertAcquire failed: %v", err)
	}
	if doc1.LeaseCount != 1 {
		t.Errorf("expected leaseCount=1 on initial acquire, got %d", doc1.LeaseCount)
	}
	if doc1.State != lease.StateCreating {
		t.Errorf("expected state CREATING, got %s", doc1.State)
	}
	if doc1.Consumer != "review" {
		t.Errorf("expected consumer review, got %s", doc1.Consumer)
	}

	// 2. Concurrent Acquire (Joiner Path)
	doc2, err := repo.UpsertAcquire(ctx, prKey, 30*time.Minute, "conversation")
	if err != nil {
		t.Fatalf("second UpsertAcquire failed: %v", err)
	}
	if doc2.LeaseCount != 2 {
		t.Errorf("expected leaseCount=2 on second acquire, got %d", doc2.LeaseCount)
	}
	if doc2.Consumer != "conversation" {
		t.Errorf("expected updated consumer, got %s", doc2.Consumer)
	}

	// 3. UpdateReady
	sbxID := "sbx-test-123"
	if err := repo.UpdateReady(ctx, prKey, sbxID); err != nil {
		t.Fatalf("UpdateReady failed: %v", err)
	}
	found, err := repo.FindByPrKey(ctx, prKey)
	if err != nil || found == nil {
		t.Fatalf("FindByPrKey failed: %v", err)
	}
	if found.State != lease.StateReady || found.SandboxID != sbxID {
		t.Errorf("expected READY state and sandboxID %s, got state=%s sandboxID=%s", sbxID, found.State, found.SandboxID)
	}

	// 4. DecrementLease
	dec1, err := repo.DecrementLease(ctx, prKey)
	if err != nil || dec1 == nil {
		t.Fatalf("DecrementLease failed: %v", err)
	}
	if dec1.LeaseCount != 1 {
		t.Errorf("expected leaseCount=1 after decrement, got %d", dec1.LeaseCount)
	}

	dec2, err := repo.DecrementLease(ctx, prKey)
	if err != nil || dec2 == nil {
		t.Fatalf("DecrementLease second failed: %v", err)
	}
	if dec2.LeaseCount != 0 {
		t.Errorf("expected leaseCount=0 after second decrement, got %d", dec2.LeaseCount)
	}

	// 5. Idle-Kill Scheduling
	killTime := time.Now().UTC().Add(30 * time.Second)
	if err := repo.SetKillAt(ctx, prKey, killTime); err != nil {
		t.Fatalf("SetKillAt failed: %v", err)
	}
	foundKill, _ := repo.FindByPrKey(ctx, prKey)
	if foundKill.KillAt == nil || !foundKill.KillAt.Equal(killTime) {
		t.Errorf("expected killAt %v, got %v", killTime, foundKill.KillAt)
	}

	// FindReadyToKill before time
	readyNone, _ := repo.FindReadyToKill(ctx, time.Now().UTC())
	if len(readyNone) != 0 {
		t.Errorf("expected 0 ready to kill, got %d", len(readyNone))
	}

	// FindReadyToKill after time
	readyKill, _ := repo.FindReadyToKill(ctx, killTime.Add(time.Second))
	if len(readyKill) != 1 {
		t.Errorf("expected 1 ready to kill, got %d", len(readyKill))
	}

	// ClearKillAt
	if err := repo.ClearKillAt(ctx, prKey); err != nil {
		t.Fatalf("ClearKillAt failed: %v", err)
	}
	foundCleared, _ := repo.FindByPrKey(ctx, prKey)
	if foundCleared.KillAt != nil {
		t.Error("expected killAt to be nil after clear")
	}

	// 6. Invalidate
	if err := repo.MarkInvalidated(ctx, prKey); err != nil {
		t.Fatalf("MarkInvalidated failed: %v", err)
	}
	foundInv, _ := repo.FindByPrKey(ctx, prKey)
	if foundInv.State != lease.StateInvalidated {
		t.Errorf("expected INVALIDATED, got %s", foundInv.State)
	}

	// 7. Cleanup claim & completion
	claimed, err := repo.ClaimCleanup(ctx, prKey, sbxID, true)
	if err != nil || claimed == nil {
		t.Fatalf("ClaimCleanup failed: %v", err)
	}
	if claimed.CleanupStatus != lease.CleanupInProgress {
		t.Errorf("expected in_progress, got %s", claimed.CleanupStatus)
	}

	// Cannot double-claim while in_progress
	secondClaim, _ := repo.ClaimCleanup(ctx, prKey, sbxID, true)
	if secondClaim != nil {
		t.Error("expected nil on second concurrent cleanup claim")
	}

	// Complete Cleanup (deletes doc)
	ok, err := repo.CompleteCleanup(ctx, prKey, sbxID)
	if err != nil || !ok {
		t.Fatalf("CompleteCleanup failed: %v", err)
	}

	afterDelete, _ := repo.FindByPrKey(ctx, prKey)
	if afterDelete != nil {
		t.Error("expected doc to be deleted after CompleteCleanup")
	}
}
