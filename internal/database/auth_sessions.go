package database

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidRefreshToken = errors.New("invalid or expired session")
	ErrRefreshTokenReuse   = errors.New("refresh token has already been consumed")
)

// ValidateAuthSession rechecks the persisted identity and session on every request.
func (r *Repository) ValidateAuthSession(ctx context.Context, userID, workspaceID uuid.UUID, sessionHash string) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, ErrRevocationStoreUnavailable
	}
	if decoded, err := hex.DecodeString(sessionHash); err != nil || len(decoded) != 32 {
		return nil, ErrInvalidRefreshToken
	}
	var user UserRecord
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT u.uuid,u.email,u.password,u.role,u.status,u.organization_id,u."createdAt",u."updatedAt"
			FROM users u JOIN auth a ON a."userUuid"=u.uuid JOIN workspaces w ON w.id=u.organization_id
			WHERE u.uuid=$1 AND u.organization_id=$2 AND a."tokenHash"=$3 AND NOT a.used
			AND a."expiryDate">clock_timestamp() AND u.status='active' AND w.status='ACTIVE'`, userID, workspaceID, sessionHash).
			Scan(&user.UUID, &user.Email, &user.Password, &user.Role, &user.Status, &user.OrganizationID, &user.CreatedAt, &user.UpdatedAt)
	})
	return &user, err
}

func lockSessionUser(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (*UserRecord, error) {
	var user UserRecord
	err := tx.QueryRow(ctx, `SELECT uuid,email,password,role,status,organization_id,"createdAt","updatedAt" FROM users WHERE uuid=$1 FOR UPDATE`, userID).
		Scan(&user.UUID, &user.Email, &user.Password, &user.Role, &user.Status, &user.OrganizationID, &user.CreatedAt, &user.UpdatedAt)
	return &user, err
}

// Hold the workspace status stable until session issuance commits. Revocation
// deliberately does not use this guard: suspended accounts must still log out.
func lockActiveSessionWorkspace(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) error {
	var active bool
	err := tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM workspaces WHERE id=$1 FOR SHARE`, workspaceID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return ErrInvalidRefreshToken
	}
	return err
}

func insertRefreshToken(ctx context.Context, tx pgx.Tx, userID uuid.UUID, token string, expiry time.Time) error {
	if token == "" || !expiry.After(time.Now()) {
		return ErrInvalidRefreshToken
	}
	_, err := tx.Exec(ctx, `INSERT INTO auth(uuid,"userUuid","tokenHash","expiryDate",used,"authProvider") VALUES ($1,$2,$3,$4,false,'credentials')`, uuid.New(), userID, hashRefreshToken(token), expiry)
	return err
}

// PersistAuthSession rejects credentials checked before a concurrent password reset.
func (r *Repository) PersistAuthSession(ctx context.Context, expected *UserRecord, token string, expiry time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return ErrRevocationStoreUnavailable
	}
	if expected == nil || expected.OrganizationID == nil {
		return ErrInvalidRefreshToken
	}
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		user, err := lockSessionUser(ctx, tx, expected.UUID)
		if err != nil {
			return err
		}
		if user.Status != "active" || (expected.Password != "" && user.Password != expected.Password) || (expected.Role != "" && !strings.EqualFold(user.Role, expected.Role)) || user.OrganizationID == nil || *user.OrganizationID != *expected.OrganizationID {
			return ErrInvalidRefreshToken
		}
		if err := lockActiveSessionWorkspace(ctx, tx, *user.OrganizationID); err != nil {
			return err
		}
		return insertRefreshToken(ctx, tx, user.UUID, token, expiry)
	})
}

// RotateRefreshToken serializes rotation, logout and reset by locking the user
// first. Spending the old token and saving the replacement commit together.
func (r *Repository) RotateRefreshToken(ctx context.Context, expected *UserRecord, oldToken, newToken string, expiry time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return ErrRevocationStoreUnavailable
	}
	if expected == nil || expected.OrganizationID == nil {
		return ErrInvalidRefreshToken
	}
	reused := false
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		user, err := lockSessionUser(ctx, tx, expected.UUID)
		if err != nil {
			return err
		}
		var used bool
		var expires time.Time
		if err := tx.QueryRow(ctx, `SELECT used,"expiryDate" FROM auth WHERE "tokenHash"=$1 AND "userUuid"=$2 FOR UPDATE`, hashRefreshToken(oldToken), user.UUID).Scan(&used, &expires); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidRefreshToken
			}
			return err
		}
		if used {
			reused = true
			return revokeUserSessions(ctx, tx, user.UUID, time.Now().Unix())
		}
		if !expires.After(time.Now()) || user.Status != "active" || (expected.Password != "" && user.Password != expected.Password) || (expected.Role != "" && !strings.EqualFold(user.Role, expected.Role)) || user.OrganizationID == nil || *user.OrganizationID != *expected.OrganizationID {
			return ErrInvalidRefreshToken
		}
		if err := lockActiveSessionWorkspace(ctx, tx, *user.OrganizationID); err != nil {
			return err
		}
		if err := insertRefreshToken(ctx, tx, user.UUID, newToken, expiry); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE auth SET used=true,"updatedAt"=now() WHERE "tokenHash"=$1 AND "userUuid"=$2`, hashRefreshToken(oldToken), user.UUID)
		return err
	})
	if err == nil && reused {
		return ErrRefreshTokenReuse
	}
	return err
}

func revokeUserSessions(ctx context.Context, tx pgx.Tx, userID uuid.UUID, cutoff int64) error {
	if _, err := tx.Exec(ctx, `UPDATE auth SET used=true,"updatedAt"=now() WHERE "userUuid"=$1`, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO revoked_access_tokens(user_id,issued_at,expires_at) VALUES($1,$2,$3)
		ON CONFLICT(user_id,issued_at) DO UPDATE SET expires_at=GREATEST(revoked_access_tokens.expires_at,EXCLUDED.expires_at)`, userID, cutoff, time.Unix(cutoff, 0).Add(accessTokenRevocationRetention))
	return err
}

func (r *Repository) RevokeUserSessions(ctx context.Context, userID uuid.UUID, cutoff int64) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return ErrRevocationStoreUnavailable
	}
	if userID == uuid.Nil || cutoff <= 0 {
		return ErrInvalidRefreshToken
	}
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		if _, err := lockSessionUser(ctx, tx, userID); err != nil {
			return err
		}
		return revokeUserSessions(ctx, tx, userID, cutoff)
	})
}

// ResetUserPassword consumes the current-hash-bound proof with the password
// change and revocation. Concurrent reuse cannot reset twice.
func (r *Repository) ResetUserPassword(ctx context.Context, userID uuid.UUID, expectedHash, newHash string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return ErrRevocationStoreUnavailable
	}
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		user, err := lockSessionUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if user.Password != expectedHash || user.Status != "active" || newHash == "" {
			return ErrInvalidRefreshToken
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET password=$1,"updatedAt"=now() WHERE uuid=$2`, newHash, userID); err != nil {
			return err
		}
		return revokeUserSessions(ctx, tx, userID, time.Now().Unix())
	})
}

func (r *Repository) ConfirmUserEmail(ctx context.Context, userID uuid.UUID, email string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return ErrRevocationStoreUnavailable
	}
	tag, err := r.execSystem(ctx, `UPDATE users SET status='active',"updatedAt"=now() WHERE uuid=$1 AND LOWER(email)=LOWER($2) AND status='pending_email'`, userID, email)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrInvalidRefreshToken
	}
	return err
}
