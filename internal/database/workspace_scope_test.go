package database

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

// TestListWorkspacesIsScopedToCaller pins the fix for the tenant leak.
//
// ListWorkspaces returns every active workspace with no per-user filter. Four
// API controllers used it to pick a default workspace or tenant name for the
// current request, which handed an authenticated user an arbitrary tenant. The
// controllers now call ListWorkspacesForUser, and this test is what stops that
// from regressing.
//
// Skips unless SCANDRIX_E2E_RUNTIME_DSN points at a live database.
func TestListWorkspacesIsScopedToCaller(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to a live database to run this test")
	}

	ctx := context.Background()
	client, err := NewClient(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()
	repo := NewRepository(client)

	// Two unrelated tenants, each with one user, so the assertion cannot pass
	// by accident on a single-tenant database.
	// Idempotent: a previous run that failed midway can leave rows behind, and
	// users.email is unique.
	cleanupTenants(t, ctx, repo)

	alice, bob, aliceWS, bobWS := seedTwoTenants(t, ctx, repo)

	aliceList, err := repo.ListWorkspacesForUser(ctx, alice)
	if err != nil {
		t.Fatalf("ListWorkspacesForUser(alice): %v", err)
	}
	if len(aliceList) != 1 || aliceList[0].ID != aliceWS {
		t.Fatalf("alice must see exactly her workspace %s, got %d rows: %+v", aliceWS, len(aliceList), aliceList)
	}

	bobList, err := repo.ListWorkspacesForUser(ctx, bob)
	if err != nil {
		t.Fatalf("ListWorkspacesForUser(bob): %v", err)
	}
	if len(bobList) != 1 || bobList[0].ID != bobWS {
		t.Fatalf("bob must see exactly his workspace %s, got %d rows", bobWS, len(bobList))
	}

	// The core of the leak: an unknown identity must see nothing. Returning
	// every workspace here is the bug this guards against.
	for _, identity := range []string{"", "   ", "nobody@nowhere.invalid"} {
		got, err := repo.ListWorkspacesForUser(ctx, identity)
		if err != nil {
			t.Fatalf("ListWorkspacesForUser(%q): %v", identity, err)
		}
		if len(got) != 0 {
			t.Fatalf("identity %q must resolve to no workspaces, got %d (this is the tenant leak)", identity, len(got))
		}
	}

	// And confirm ListWorkspaces really is unscoped, so the test above is
	// actually distinguishing the two functions.
	all, err := repo.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(all) <= len(aliceList) {
		t.Fatalf("ListWorkspaces should return more than a single tenant's view; got %d vs %d", len(all), len(aliceList))
	}

	cleanupTenants(t, ctx, repo)
}

// seedTwoTenants creates two tenants with one user each and returns the emails
// and workspace ids.
func seedTwoTenants(t *testing.T, ctx context.Context, repo *Repository) (alice, bob string, aliceWS, bobWS uuid.UUID) {
	t.Helper()
	alice, bob = "rls-alice@scandrix.test", "rls-bob@scandrix.test"
	aliceWS = newTenant(t, ctx, repo, alice, "rls-alice")
	bobWS = newTenant(t, ctx, repo, bob, "rls-bob")
	return alice, bob, aliceWS, bobWS
}

func newTenant(t *testing.T, ctx context.Context, repo *Repository, email, slug string) uuid.UUID {
	t.Helper()
	wsID := uuid.New()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: wsID, Slug: slug + "-" + wsID.String()[:8], Name: slug, Status: "ACTIVE",
	}); err != nil {
		t.Fatalf("seed workspace %s: %v", slug, err)
	}
	// Only users is seeded: ListWorkspacesForUser joins workspaces to users.
	// (account_profiles is deliberately not written here - its RLS policy has no
	// system-worker branch, so a seed insert is rejected under least privilege.)
	if _, err := clientExec(ctx, repo, `INSERT INTO users (uuid, email, password, role, status, organization_id, "createdAt", "updatedAt")
		VALUES (gen_random_uuid(), $1, 'not-a-real-hash', 'member', 'active', $2, now(), now())`, email, wsID); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return wsID
}

// clientExec runs a seeding statement as a system worker, because seeding has
// no request tenant to run under.
func clientExec(ctx context.Context, repo *Repository, sql string, args ...any) (int64, error) {
	var n int64
	err := repo.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}

// cleanupTenants removes the seeded rows.
//
// The DELETEs must run as a system worker. A bare pool call matches zero rows
// under RLS, so the cleanup would silently do nothing and the next run would
// fail on the unique constraint on users.email.
func cleanupTenants(t *testing.T, ctx context.Context, repo *Repository) {
	t.Helper()
	stmts := []string{
		`DELETE FROM users WHERE email LIKE 'rls-%@scandrix.test'`,
		`DELETE FROM account_profiles WHERE email LIKE 'rls-%@scandrix.test'`,
		`DELETE FROM workspaces WHERE slug LIKE 'rls-alice-%' OR slug LIKE 'rls-bob-%'`,
	}
	for _, sql := range stmts {
		if err := repo.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql)
			return err
		}); err != nil {
			t.Logf("cleanup %q: %v", sql, err)
		}
	}
}
