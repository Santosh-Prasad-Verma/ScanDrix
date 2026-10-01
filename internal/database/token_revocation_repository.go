package database

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Access-token revocation (AUDIT_REMEDIATION.md F-18).
//
// A row records a cutoff, not a single token: any access token for that user
// whose issued-at is less than or equal to the cutoff is rejected. Logout
// writes one cutoff, which invalidates every session token outstanding at that
// instant -- including copies already made by someone else -- without having to
// enumerate them.
//
// Tokens minted after the cutoff are unaffected, so a refresh racing with the
// logout produces a genuinely new session rather than being swept up by it.
//
// The table is RLS-forced with a system-worker-only policy (migrations/041),
// because the lookup runs before the middleware has established any tenant
// context. Every access below therefore goes through the system executor.

// accessTokenRevocationRetention is how long a revocation row must be kept.
//
// A cutoff row has to outlive the newest token it covers, which is the token
// issued exactly at the cutoff. That token lives for one access-token lifetime
// (auth.DefaultAccessTokenTTL, 15 minutes), so the row is retained for the same
// span. The value is duplicated rather than imported because internal/auth does
// not depend on internal/database and this package must not take on that
// dependency; TestAccessTokenRevocationRetentionMatchesAccessTokenTTL in
// token_revocation_repository_test.go fails if the two ever drift apart.
const accessTokenRevocationRetention = 15 * time.Minute

// ErrRevocationStoreUnavailable is returned when the backing pool is missing.
// Callers must treat it as "cannot prove the token is still good" rather than
// silently treating the token as valid.
var ErrRevocationStoreUnavailable = errors.New("token revocation store unavailable")

// RevokeAccessTokensUpTo records that every access token for userID issued at
// or before cutoffIssuedAt is revoked. The row is retained only until the last
// token it could affect has expired, so the sweep in
// PurgeExpiredAccessTokenRevocations can bound the table.
//
// cutoffIssuedAt is a JWT issued-at value (seconds since the Unix epoch).
func (r *Repository) RevokeAccessTokensUpTo(ctx context.Context, userID uuid.UUID, cutoffIssuedAt int64) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return ErrRevocationStoreUnavailable
	}
	if userID == uuid.Nil {
		return errors.New("user id is required to revoke access tokens")
	}
	if cutoffIssuedAt <= 0 {
		return errors.New("revocation cutoff must be a positive unix timestamp")
	}

	// A token issued exactly at the cutoff is still outstanding, so the row has
	// to outlive cutoffIssuedAt plus a full access-token lifetime. Any token
	// older than that expired on its own.
	expiresAt := time.Unix(cutoffIssuedAt, 0).Add(accessTokenRevocationRetention)

	query := `
		INSERT INTO revoked_access_tokens (user_id, issued_at, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, issued_at) DO UPDATE
			SET expires_at = GREATEST(revoked_access_tokens.expires_at, EXCLUDED.expires_at);
	`
	_, err := r.execSystem(ctx, query, userID, cutoffIssuedAt, expiresAt)
	return err
}

// IsAccessTokenRevoked reports whether the token identified by userID and
// issuedAt has been revoked.
//
// A store error is returned rather than collapsed into a boolean. The caller
// decides the failure mode; silently answering "not revoked" on a database
// error would turn an outage into a security hole.
func (r *Repository) IsAccessTokenRevoked(ctx context.Context, userID uuid.UUID, issuedAt int64) (bool, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return false, ErrRevocationStoreUnavailable
	}
	if userID == uuid.Nil || issuedAt <= 0 {
		return false, errors.New("user id and issued-at are required")
	}

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM revoked_access_tokens
			WHERE user_id = $1
			  AND issued_at >= $2
			  AND expires_at > now()
		);
	`
	var revoked bool
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, userID, issuedAt).Scan(&revoked)
	})
	if err != nil {
		return false, err
	}
	return revoked, nil
}

// PurgeExpiredAccessTokenRevocations drops rows whose covered tokens have all
// expired. Without this the table grows by one row per logout forever.
func (r *Repository) PurgeExpiredAccessTokenRevocations(ctx context.Context) (int64, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, ErrRevocationStoreUnavailable
	}
	var tag pgconn.CommandTag
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		var e error
		tag, e = tx.Exec(ctx, `DELETE FROM revoked_access_tokens WHERE expires_at <= now();`)
		return e
	})
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
