package lease_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/lease"
	"github.com/scandrix/backend/internal/sandbox/null"
)

func TestLeaseManager_CreatorAndJoiner(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	nullProvider := null.NewNullSandboxProvider()

	mgr := lease.NewSandboxLeaseManager(nullProvider, repo, nil)

	orgID := uuid.New().String()
	prKey, err := contracts.BuildPrKey(orgID, "test-repo", 100)
	if err != nil {
		t.Fatalf("BuildPrKey failed: %v", err)
	}

	// 1. First caller: Creator path
	res1, err := mgr.Acquire(ctx, prKey, "review", 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("Acquire creator failed: %v", err)
	}
	if !res1.WasCreated {
		t.Error("expected res1.WasCreated = true")
	}
	if res1.LeaseID == "" {
		t.Error("expected non-empty leaseID")
	}

	// 2. Second caller: Joiner path (doc is already READY)
	res2, err := mgr.Acquire(ctx, prKey, "chat", 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("Acquire joiner failed: %v", err)
	}
	if res2.WasCreated {
		t.Error("expected res2.WasCreated = false for joiner")
	}
	if res2.SandboxID != res1.SandboxID {
		t.Errorf("expected same sandboxID, got %s vs %s", res1.SandboxID, res2.SandboxID)
	}

	// 3. Release first lease (lease count becomes 1, not 0)
	if err := mgr.Release(ctx, res1.LeaseID, nil); err != nil {
		t.Fatalf("Release res1 failed: %v", err)
	}
	doc, _ := repo.FindByPrKey(ctx, prKey)
	if doc.LeaseCount != 1 {
		t.Errorf("expected leaseCount=1 after first release, got %d", doc.LeaseCount)
	}
	if doc.KillAt != nil {
		t.Error("expected KillAt to be nil when leases remain active")
	}

	// 4. Release second lease (lease count becomes 0 -> idle-kill scheduled)
	if err := mgr.Release(ctx, res2.LeaseID, &contracts.ReleaseOptions{IdleTimeout: 10 * time.Second}); err != nil {
		t.Fatalf("Release res2 failed: %v", err)
	}
	docAfter, _ := repo.FindByPrKey(ctx, prKey)
	if docAfter.LeaseCount != 0 {
		t.Errorf("expected leaseCount=0 after second release, got %d", docAfter.LeaseCount)
	}
	if docAfter.KillAt == nil {
		t.Error("expected KillAt to be set when last lease released")
	}
}

func TestLeaseManager_LocalImmediateCleanup(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	mgr := lease.NewSandboxLeaseManager(nil, repo, nil)

	// Create real temporary local sandbox directory
	tmpDir := os.TempDir()
	localSandboxDir := filepath.Join(tmpDir, "scandrix-sandbox-cleanup-test")
	if err := os.MkdirAll(localSandboxDir, 0700); err != nil {
		t.Fatalf("failed creating local test dir: %v", err)
	}

	orgID := uuid.New().String()
	prKey, _ := contracts.BuildPrKey(orgID, "local-repo", 200)

	// Manually set existing READY lease pointing to local sandbox
	doc, _ := repo.UpsertAcquire(ctx, prKey, 10*time.Minute, "review")
	_ = repo.UpdateReady(ctx, prKey, localSandboxDir)

	// Acquire via joiner
	res, err := mgr.Acquire(ctx, prKey, "review", 10*time.Minute, nil)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Release: with leaseCount=0 and local path, it must be cleaned up immediately!
	_, _ = repo.DecrementLease(ctx, prKey) // decrement initial doc count so this release drops to 0
	_ = doc


	if err := mgr.Release(ctx, res.LeaseID, nil); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Directory should be deleted
	if _, err := os.Stat(localSandboxDir); !os.IsNotExist(err) {
		t.Errorf("expected local sandbox dir %s to be deleted, but it still exists", localSandboxDir)
	}

	// Lease record should be deleted
	afterDoc, _ := repo.FindByPrKey(ctx, prKey)
	if afterDoc != nil {
		t.Errorf("expected lease doc to be deleted after local cleanup, got: %+v", afterDoc)
	}
}

func TestLeaseManager_MidCreateInvalidation(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()

	orgID := uuid.New().String()
	prKey, _ := contracts.BuildPrKey(orgID, "mid-repo", 300)

	// Pre-invalidate
	doc, _ := repo.UpsertAcquire(ctx, prKey, 10*time.Minute, "creator")
	_ = repo.MarkInvalidated(ctx, prKey)
	_ = doc

	mgr := lease.NewSandboxLeaseManager(nil, repo, nil)

	// Acquiring on INVALIDATED doc should return SandboxInvalidatedError
	_, err := mgr.Acquire(ctx, prKey, "consumer", 10*time.Minute, nil)
	if err == nil {
		t.Fatal("expected SandboxInvalidatedError, got nil")
	}

	var invErr *contracts.SandboxInvalidatedError
	if !errors.As(err, &invErr) {
		t.Fatalf("expected *contracts.SandboxInvalidatedError, got: %T (%v)", err, err)
	}
}

func TestLeaseManager_StaleConnectionRecovery(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	nullProvider := null.NewNullSandboxProvider()

	mgr := lease.NewSandboxLeaseManager(nullProvider, repo, nil)

	orgID := uuid.New().String()
	prKey, _ := contracts.BuildPrKey(orgID, "stale-repo", 400)

	// Seed existing lease with dead sandbox ID
	_, _ = repo.UpsertAcquire(ctx, prKey, 10*time.Minute, "first")
	_ = repo.UpdateReady(ctx, prKey, "sbx-stale-999")

	// Mock hook that fails on "sbx-stale-999" (simulating 404 from E2B)
	// and succeeds on newly created sandboxes
	callCount := 0
	mgr.SetConnectExistingHook(func(ctx context.Context, sandboxID string) (contracts.SandboxInstance, error) {
		callCount++
		if sandboxID == "sbx-stale-999" {
			return nil, errors.New("404 sandbox not found")
		}
		return null.NewNullSandboxInstance(), nil
	})

	// Acquire should detect stale sandbox, delete old lease, cold-create fresh sandbox, and succeed!
	res, err := mgr.Acquire(ctx, prKey, "joiner", 10*time.Minute, nil)
	if err != nil {
		t.Fatalf("Acquire with stale recovery failed: %v", err)
	}

	if !res.WasCreated {
		t.Error("expected res.WasCreated = true after recovering from stale sandbox")
	}
}
