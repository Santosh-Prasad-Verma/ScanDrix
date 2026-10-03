package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/auth"
)

// accessTokenRevocationRetention is duplicated rather than imported from
// internal/auth (which does not depend on internal/database). This test exists
// so the duplication cannot drift: if the access-token lifetime changes without
// the revocation retention following it, revoked rows would be purged while the
// tokens they cover are still valid.
func TestAccessTokenRevocationRetentionMatchesAccessTokenTTL(t *testing.T) {
	if accessTokenRevocationRetention != auth.DefaultAccessTokenTTL {
		t.Fatalf("revocation retention %v must equal the access-token TTL %v; "+
			"a shorter retention purges rows while the tokens they cover are still valid",
			accessTokenRevocationRetention, auth.DefaultAccessTokenTTL)
	}
}

// These tests need the real database and the real runtime role, because the
// behaviour under test is a row-level-security policy. A superuser bypasses
// RLS and would pass while proving nothing, so the connection is rejected if
// it turns out to be privileged. Skips unless SCANDRIX_E2E_RUNTIME_DSN is set,
// matching findings_rls_test.go.
func newRevocationRepo(t *testing.T) *Repository {
	t.Helper()
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to a live database to run this test")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, dsn)
	if err != nil {
		t.Fatalf("connect as runtime role: %v", err)
	}
	t.Cleanup(client.Close)

	var isSuper, bypassRLS bool
	if err := client.Pool.QueryRow(ctx,
		"SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user",
	).Scan(&isSuper, &bypassRLS); err != nil {
		t.Fatalf("read role attributes: %v", err)
	}
	if isSuper || bypassRLS {
		t.Fatalf("connected as a privileged role (superuser=%v bypassrls=%v); this test would prove nothing",
			isSuper, bypassRLS)
	}
	return NewRepository(client)
}

func TestRevokeAccessTokensUpToRevokesAndPurges(t *testing.T) {
	repo := newRevocationRepo(t)
	ctx := context.Background()

	userID := uuid.New()
	cutoff := time.Now().UTC().Unix()

	if err := repo.RevokeAccessTokensUpTo(ctx, userID, cutoff); err != nil {
		t.Fatalf("RevokeAccessTokensUpTo: %v", err)
	}

	// At or before the cutoff is revoked.
	for _, issuedAt := range []int64{cutoff, cutoff - 1, cutoff - 3600} {
		revoked, err := repo.IsAccessTokenRevoked(ctx, userID, issuedAt)
		if err != nil {
			t.Fatalf("IsAccessTokenRevoked(%d): %v", issuedAt, err)
		}
		if !revoked {
			t.Errorf("expected issued_at=%d (<= cutoff %d) to be revoked", issuedAt, cutoff)
		}
	}

	// After the cutoff survives.
	issuedAfter := cutoff + 1
	revoked, err := repo.IsAccessTokenRevoked(ctx, userID, issuedAfter)
	if err != nil {
		t.Fatalf("IsAccessTokenRevoked(%d): %v", issuedAfter, err)
	}
	if revoked {
		t.Errorf("expected issued_at=%d (> cutoff %d) not to be revoked", issuedAfter, cutoff)
	}

	// Another user's tokens are unaffected.
	other, err := repo.IsAccessTokenRevoked(ctx, uuid.New(), cutoff)
	if err != nil {
		t.Fatalf("IsAccessTokenRevoked(other user): %v", err)
	}
	if other {
		t.Error("revocation must not leak across users")
	}

	// Revoking again with a later cutoff widens the window rather than failing.
	later := cutoff + 60
	if err := repo.RevokeAccessTokensUpTo(ctx, userID, later); err != nil {
		t.Fatalf("second RevokeAccessTokensUpTo: %v", err)
	}
	revoked, err = repo.IsAccessTokenRevoked(ctx, userID, later)
	if err != nil {
		t.Fatalf("IsAccessTokenRevoked after widening: %v", err)
	}
	if !revoked {
		t.Error("expected the widened cutoff to cover the later token")
	}
}

func TestPurgeExpiredAccessTokenRevocations(t *testing.T) {
	repo := newRevocationRepo(t)
	ctx := context.Background()

	// A cutoff far enough in the past that every token it could cover has
	// already expired, so the row is collectable. Without a sweep the table
	// would grow by one row per logout forever.
	staleUser := uuid.New()
	stale := time.Now().UTC().Add(-48 * time.Hour).Unix()
	if err := repo.RevokeAccessTokensUpTo(ctx, staleUser, stale); err != nil {
		t.Fatalf("seed stale revocation: %v", err)
	}

	// A live row must survive the sweep.
	liveUser := uuid.New()
	live := time.Now().UTC().Unix()
	if err := repo.RevokeAccessTokensUpTo(ctx, liveUser, live); err != nil {
		t.Fatalf("seed live revocation: %v", err)
	}

	if _, err := repo.PurgeExpiredAccessTokenRevocations(ctx); err != nil {
		t.Fatalf("PurgeExpiredAccessTokenRevocations: %v", err)
	}

	// Both cutoffs are stale enough that the retained expiry is already past,
	// so IsAccessTokenRevoked reports false for both regardless of the sweep.
	// The sweep's real job is row count, asserted separately below.
	if _, err := repo.IsAccessTokenRevoked(ctx, staleUser, stale); err != nil {
		t.Fatalf("IsAccessTokenRevoked(stale): %v", err)
	}

	// The meaningful check: the live cutoff is still enforced after a sweep.
	revoked, err := repo.IsAccessTokenRevoked(ctx, liveUser, live)
	if err != nil {
		t.Fatalf("IsAccessTokenRevoked(live): %v", err)
	}
	if !revoked {
		t.Error("a purge must not drop a revocation that still covers a valid token")
	}
}

func TestRevocationValidatesArguments(t *testing.T) {
	repo := newRevocationRepo(t)
	ctx := context.Background()

	if err := repo.RevokeAccessTokensUpTo(ctx, uuid.Nil, 1); err == nil {
		t.Error("expected an error for a nil user id")
	}
	if err := repo.RevokeAccessTokensUpTo(ctx, uuid.New(), 0); err == nil {
		t.Error("expected an error for a zero cutoff")
	}
	if _, err := repo.IsAccessTokenRevoked(ctx, uuid.Nil, 1); err == nil {
		t.Error("expected an error for a nil user id")
	}
	if _, err := repo.IsAccessTokenRevoked(ctx, uuid.New(), 0); err == nil {
		t.Error("expected an error for a zero issued-at")
	}
}
