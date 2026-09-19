package lease_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/lease"
)

func TestSandboxLeaseReaper_ReapExpired(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	reaper := lease.NewSandboxLeaseReaper(repo, nil)

	var killedMu sync.Mutex
	var killedSandboxes []string
	reaper.SetKillHook(func(ctx context.Context, sandboxID string) error {
		killedMu.Lock()
		defer killedMu.Unlock()
		killedSandboxes = append(killedSandboxes, sandboxID)
		return nil
	})

	orgID := uuid.New().String()
	prKeyExpired, _ := contracts.BuildPrKey(orgID, "exp-repo", 1)
	prKeyActive, _ := contracts.BuildPrKey(orgID, "act-repo", 2)

	// Seed expired lease (expired 5 minutes ago)
	docExp, _ := repo.UpsertAcquire(ctx, prKeyExpired, -5*time.Minute, "review")
	_ = repo.UpdateReady(ctx, prKeyExpired, "sbx-exp-999")
	_ = docExp

	// Seed active lease (expires in 30 minutes)
	docAct, _ := repo.UpsertAcquire(ctx, prKeyActive, 30*time.Minute, "review")
	_ = repo.UpdateReady(ctx, prKeyActive, "sbx-act-111")
	_ = docAct

	// Run reaper
	if err := reaper.ReapExpiredLeases(ctx); err != nil {
		t.Fatalf("ReapExpiredLeases failed: %v", err)
	}

	// Verify expired lease was killed and deleted
	killedMu.Lock()
	if len(killedSandboxes) != 1 || killedSandboxes[0] != "sbx-exp-999" {
		t.Errorf("expected sbx-exp-999 to be killed, got: %v", killedSandboxes)
	}
	killedMu.Unlock()

	foundExp, _ := repo.FindByPrKey(ctx, prKeyExpired)
	if foundExp != nil {
		t.Errorf("expected expired lease to be deleted, found: %+v", foundExp)
	}

	// Verify active lease was NOT touched
	foundAct, _ := repo.FindByPrKey(ctx, prKeyActive)
	if foundAct == nil {
		t.Error("expected active lease to remain in repository")
	}
}

func TestSandboxLeaseReaper_KillIdleSandboxes(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	reaper := lease.NewSandboxLeaseReaper(repo, nil)

	var killedMu sync.Mutex
	var killedSandboxes []string
	reaper.SetKillHook(func(ctx context.Context, sandboxID string) error {
		killedMu.Lock()
		defer killedMu.Unlock()
		killedSandboxes = append(killedSandboxes, sandboxID)
		return nil
	})

	orgID := uuid.New().String()
	prKeyIdle, _ := contracts.BuildPrKey(orgID, "idle-repo", 10)
	prKeyBusy, _ := contracts.BuildPrKey(orgID, "busy-repo", 20)

	// Seed idle lease (killAt in the past)
	_, _ = repo.UpsertAcquire(ctx, prKeyIdle, 30*time.Minute, "review")
	_ = repo.UpdateReady(ctx, prKeyIdle, "sbx-idle-888")
	_ = repo.SetKillAt(ctx, prKeyIdle, time.Now().UTC().Add(-10*time.Second))

	// Seed busy lease (no killAt set)
	_, _ = repo.UpsertAcquire(ctx, prKeyBusy, 30*time.Minute, "review")
	_ = repo.UpdateReady(ctx, prKeyBusy, "sbx-busy-777")

	// Run idle kill
	if err := reaper.KillIdleSandboxes(ctx); err != nil {
		t.Fatalf("KillIdleSandboxes failed: %v", err)
	}

	killedMu.Lock()
	if len(killedSandboxes) != 1 || killedSandboxes[0] != "sbx-idle-888" {
		t.Errorf("expected sbx-idle-888 killed, got: %v", killedSandboxes)
	}
	killedMu.Unlock()

	foundIdle, _ := repo.FindByPrKey(ctx, prKeyIdle)
	if foundIdle != nil {
		t.Error("expected idle lease to be deleted")
	}

	foundBusy, _ := repo.FindByPrKey(ctx, prKeyBusy)
	if foundBusy == nil {
		t.Error("expected busy lease to remain")
	}
}

func TestSandboxLeaseReaper_LocalSandboxReap(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	reaper := lease.NewSandboxLeaseReaper(repo, nil)

	tmpDir := os.TempDir()
	localDir := filepath.Join(tmpDir, "scandrix-sandbox-reaper-test")
	if err := os.MkdirAll(localDir, 0700); err != nil {
		t.Fatalf("failed creating local test dir: %v", err)
	}

	orgID := uuid.New().String()
	prKey, _ := contracts.BuildPrKey(orgID, "local-repo", 55)

	_, _ = repo.UpsertAcquire(ctx, prKey, -1*time.Minute, "review")
	_ = repo.UpdateReady(ctx, prKey, localDir)

	if err := reaper.ReapExpiredLeases(ctx); err != nil {
		t.Fatalf("ReapExpiredLeases failed: %v", err)
	}

	if _, err := os.Stat(localDir); !os.IsNotExist(err) {
		t.Errorf("expected local sandbox dir %s to be reaped", localDir)
	}

	found, _ := repo.FindByPrKey(ctx, prKey)
	if found != nil {
		t.Error("expected lease to be deleted after reap")
	}
}

func TestCronJobAdapters(t *testing.T) {
	repo := lease.NewMemorySandboxLeaseRepository()
	reaper := lease.NewSandboxLeaseReaper(repo, nil)

	reaperJob := lease.NewReaperCronJob(reaper)
	if reaperJob.Name() != "SandboxLeaseReaper" {
		t.Errorf("expected job name SandboxLeaseReaper, got %s", reaperJob.Name())
	}
	if reaperJob.Interval() != 5*time.Minute {
		t.Errorf("expected interval 5m, got %v", reaperJob.Interval())
	}

	idleJob := lease.NewIdleKillCronJob(reaper)
	if idleJob.Name() != "SandboxIdleKill" {
		t.Errorf("expected job name SandboxIdleKill, got %s", idleJob.Name())
	}
	if idleJob.Interval() != 30*time.Second {
		t.Errorf("expected interval 30s, got %v", idleJob.Interval())
	}
}
