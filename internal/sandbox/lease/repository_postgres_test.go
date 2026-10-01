// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package lease

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// These exercise the Postgres lease repository against a real database. The unit
// tests in this package all use the in-memory repository, so without these the
// SQL — including the RLS system-worker scoping it depends on — is never run.
//
// Set SCANDRIX_TEST_DATABASE_URL to enable them. They are skipped otherwise, so
// the package still tests clean without a database.

func testClient(t *testing.T) *database.Client {
	t.Helper()
	url := os.Getenv("SCANDRIX_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SCANDRIX_TEST_DATABASE_URL not set; skipping Postgres lease tests")
	}
	client, err := database.NewClient(context.Background(), url)
	if err != nil {
		t.Fatalf("failed connecting to the test database: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestPgLeaseRepository_AcquireAndReadBack(t *testing.T) {
	client := testClient(t)
	repo := NewPgSandboxLeaseRepository(client)
	ctx := context.Background()

	// pr_key shape is <orgUUID|trial>:<repositoryId>:<prNumber>.
	prKey := "trial:repo-123:1"
	lease, err := repo.UpsertAcquire(ctx, prKey, time.Minute, "review")
	if err != nil {
		t.Fatalf("UpsertAcquire failed: %v", err)
	}
	if lease.PrKey != prKey {
		t.Fatalf("expected pr_key %q, got %q", prKey, lease.PrKey)
	}
	if lease.LeaseCount != 1 {
		t.Fatalf("expected lease_count 1, got %d", lease.LeaseCount)
	}

	// scanRowAsSystem must find the row it just wrote.
	got, err := repo.FindByPrKey(ctx, prKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil for a lease that was just acquired")
	}
	if got.RepositoryID != lease.RepositoryID {
		t.Fatalf("expected repository_id %q, got %q", lease.RepositoryID, got.RepositoryID)
	}

	t.Cleanup(func() { _ = repo.Delete(ctx, prKey) })
}

func TestPgLeaseRepository_GetMissingLeaseReturnsNil(t *testing.T) {
	client := testClient(t)
	repo := NewPgSandboxLeaseRepository(client)

	got, err := repo.FindByPrKey(context.Background(), "trial:repo-999:9")
	if err != nil {
		t.Fatalf("a missing lease must not be an error, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for an unknown lease, got %+v", got)
	}
}

func TestPgLeaseRepository_LeaseCountLifecycle(t *testing.T) {
	client := testClient(t)
	repo := NewPgSandboxLeaseRepository(client)
	ctx := context.Background()

	prKey := "trial:repo-555:2"
	if _, err := repo.UpsertAcquire(ctx, prKey, time.Minute, "review"); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(ctx, prKey) })

	if _, err := repo.UpsertAcquire(ctx, prKey, time.Minute, "review"); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	got, err := repo.FindByPrKey(ctx, prKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil || got.LeaseCount != 2 {
		t.Fatalf("expected lease_count 2, got %+v", got)
	}

	dec, err := repo.DecrementLease(ctx, prKey)
	if err != nil {
		t.Fatalf("DecrementLease failed: %v", err)
	}
	if dec == nil || dec.LeaseCount != 1 {
		t.Fatalf("expected lease_count 1 after decrement, got %+v", dec)
	}
}

func TestPgLeaseRepository_FindReadyToKillReturnsRows(t *testing.T) {
	client := testClient(t)
	repo := NewPgSandboxLeaseRepository(client)
	ctx := context.Background()

	// queryAsSystem must be able to stream rows inside the transaction.
	// FindReadyToKill only returns leases that already have a sandbox and are
	// past their kill_at, so the lease is marked ready first.
	prKey := "trial:repo-777:3"
	if _, err := repo.UpsertAcquire(ctx, prKey, time.Hour, "review"); err != nil {
		t.Fatalf("UpsertAcquire failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(ctx, prKey) })

	if err := repo.UpdateReady(ctx, prKey, "sbx-test-777"); err != nil {
		t.Fatalf("UpdateReady failed: %v", err)
	}
	if err := repo.SetKillAt(ctx, prKey, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("SetKillAt failed: %v", err)
	}

	ready, err := repo.FindReadyToKill(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("FindReadyToKill failed: %v", err)
	}
	found := false
	for _, l := range ready {
		if l.PrKey == prKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %q among the ready-to-kill leases, got %d rows", prKey, len(ready))
	}
}
