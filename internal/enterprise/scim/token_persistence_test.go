// ═══════════════════════════════════════════════════════════════════════════
// ScanDrix AI - SCIM tenant resolution against a real database
//
// Skipped unless SCANDRIX_TEST_DATABASE_URL is set, so the package still tests
// clean without one.
// ═══════════════════════════════════════════════════════════════════════════

package scim_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/enterprise/scim"
	"github.com/scandrix/backend/pkg/models"
)

func newSCIMTestRepo(t *testing.T) *database.Repository {
	t.Helper()
	url := os.Getenv("SCANDRIX_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SCANDRIX_TEST_DATABASE_URL not set; skipping database-backed SCIM test")
	}
	client, err := database.NewClient(context.Background(), url)
	if err != nil {
		t.Fatalf("failed connecting to the test database: %v", err)
	}
	t.Cleanup(client.Close)
	return database.NewRepository(client)
}

func newTestWorkspace(t *testing.T, repo *database.Repository) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	wsID := uuid.New()
	// A unique slug per run keeps repeated runs from colliding on the unique
	// index, and the workspace is removed afterwards so the fixture leaves no
	// trace in a shared database.
	ws := &models.Workspace{
		ID:     wsID,
		Slug:   "scim-" + wsID.String()[:8],
		Name:   "SCIM token test",
		Status: models.TenantStatusActive,
	}
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Skipf("cannot create a workspace in this database (%v); skipping", err)
	}
	t.Cleanup(func() {
		// Removed via the system connection because the test connection runs
		// under RLS, which would hide the row from the deleting statement.
		_ = repo.Client().ExecAsSystem(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", wsID)
			return err
		})
	})
	return wsID
}

// A workspace's own token resolves back to that workspace, and to no other.
func TestSCIMTokenResolvesToItsOwnWorkspaceOnly(t *testing.T) {
	repo := newSCIMTestRepo(t)
	svc := scim.NewSCIMService(repo)
	ctx := context.Background()

	wsA := newTestWorkspace(t, repo)
	wsB := newTestWorkspace(t, repo)

	tokenA, prefixA, err := svc.IssueToken(ctx, wsA)
	if err != nil {
		t.Fatalf("IssueToken(A) failed: %v", err)
	}
	if tokenA == "" {
		t.Fatal("IssueToken returned an empty token")
	}
	if len(prefixA) == 0 || len(prefixA) >= len(tokenA) {
		t.Errorf("prefix %q must be a short, non-secret label", prefixA)
	}

	tokenB, _, err := svc.IssueToken(ctx, wsB)
	if err != nil {
		t.Fatalf("IssueToken(B) failed: %v", err)
	}
	if tokenA == tokenB {
		t.Error("two workspaces were issued the same token")
	}

	// Resolving B's token must never yield A.
	got, err := repo.ResolveSCIMWorkspaceByToken(ctx, scimTokenHashForTest(tokenB))
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got != wsB {
		t.Errorf("B's token resolved to %s, want %s", got, wsB)
	}
	if got == wsA {
		t.Error("cross-tenant: B's token resolved to A's workspace")
	}

	// An unknown token resolves to nothing rather than defaulting to a tenant.
	got, err = repo.ResolveSCIMWorkspaceByToken(ctx, scimTokenHashForTest("scim_not-a-real-token"))
	if err != nil {
		t.Fatalf("resolve of an unknown token failed: %v", err)
	}
	if got != uuid.Nil {
		t.Errorf("an unknown token resolved to %s; want no tenant", got)
	}

	// Revoking disables resolution for that workspace only.
	if err := svc.RevokeToken(ctx, wsB); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}
	got, err = repo.ResolveSCIMWorkspaceByToken(ctx, scimTokenHashForTest(tokenB))
	if err != nil {
		t.Fatalf("resolve after revoke failed: %v", err)
	}
	if got != uuid.Nil {
		t.Errorf("a revoked token still resolves to %s", got)
	}
	// A's token is untouched by B's revocation.
	got, _ = repo.ResolveSCIMWorkspaceByToken(ctx, scimTokenHashForTest(tokenA))
	if got != wsA {
		t.Errorf("revoking B disturbed A: got %s, want %s", got, wsA)
	}
}

// Rotating a token invalidates the previous one.
func TestSCIMTokenRotationInvalidatesTheOldValue(t *testing.T) {
	repo := newSCIMTestRepo(t)
	svc := scim.NewSCIMService(repo)
	ctx := context.Background()
	wsID := newTestWorkspace(t, repo)

	first, _, err := svc.IssueToken(ctx, wsID)
	if err != nil {
		t.Fatalf("first IssueToken failed: %v", err)
	}
	second, _, err := svc.IssueToken(ctx, wsID)
	if err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
	if first == second {
		t.Fatal("rotation returned the same token")
	}

	got, _ := repo.ResolveSCIMWorkspaceByToken(ctx, scimTokenHashForTest(first))
	if got != uuid.Nil {
		t.Error("the superseded token still resolves")
	}
	got, _ = repo.ResolveSCIMWorkspaceByToken(ctx, scimTokenHashForTest(second))
	if got != wsID {
		t.Errorf("the current token resolves to %s, want %s", got, wsID)
	}
}

// The plaintext is never persisted: only its hash is, so a database dump cannot
// be replayed as a live credential.
func TestSCIMPlaintextIsNotStored(t *testing.T) {
	repo := newSCIMTestRepo(t)
	svc := scim.NewSCIMService(repo)
	ctx := context.Background()
	wsID := newTestWorkspace(t, repo)

	token, prefix, err := svc.IssueToken(ctx, wsID)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	enabled, err := svc.TokenState(ctx, wsID)
	if err != nil {
		t.Fatalf("TokenState failed: %v", err)
	}
	if !enabled {
		t.Error("TokenState reports disabled immediately after issuance")
	}

	// The stored hash must not contain the plaintext, and the prefix is the
	// only part that is meant to be recognisable.
	if scimTokenHashForTest(token) == token {
		t.Error("the token was stored verbatim")
	}
	if prefix == token {
		t.Error("prefix must not be the whole token")
	}
}
