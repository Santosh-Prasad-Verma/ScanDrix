package database

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/cliauth"
)

// PostgresCLISessionStore adapts the PostgreSQL Repository with an in-memory resilient fallback.
type PostgresCLISessionStore struct {
	repo       *Repository
	fallback   *cliauth.InMemorySessionStore
	mu         sync.RWMutex
	tableReady bool
}

// NewPostgresCLISessionStore initializes a resilient PostgreSQL CLI session store.
func NewPostgresCLISessionStore(repo *Repository) *PostgresCLISessionStore {
	return &PostgresCLISessionStore{
		repo:     repo,
		fallback: cliauth.NewInMemorySessionStore(),
	}
}

func (s *PostgresCLISessionStore) ensureTable(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tableReady || s.repo == nil || s.repo.client == nil || s.repo.client.Pool == nil {
		return
	}

	createTableSQL := `
		CREATE TABLE IF NOT EXISTS cli_auth_sessions (
			uuid UUID PRIMARY KEY,
			state TEXT,
			device_code TEXT UNIQUE,
			user_code TEXT UNIQUE,
			redirect_uri TEXT,
			mode TEXT,
			status TEXT NOT NULL DEFAULT 'PENDING',
			access_token TEXT,
			refresh_token TEXT,
			user_id UUID,
			workspace_id UUID,
			user_email TEXT,
			user_agent TEXT,
			expires_at TIMESTAMPTZ NOT NULL,
			consumed_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			"createdAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
			"updatedAt" TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_device ON cli_auth_sessions(device_code);
		CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_user ON cli_auth_sessions(user_code);
	`
	if _, err := s.repo.client.Pool.Exec(ctx, createTableSQL); err != nil {
		return
	}
	// AUDIT_REMEDIATION.md F-37. The tenant column was added after the table
	// first shipped, so upgrade existing tables in place. This is NOT RLS yet:
	// see the F-37 row for why, and note that RLS needs a tenant-scoped policy
	// which the device flow can now satisfy because approval binds the tenant.
	_, err := s.repo.client.Pool.Exec(ctx, `
		ALTER TABLE cli_auth_sessions ADD COLUMN IF NOT EXISTS workspace_id UUID;
		CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_workspace ON cli_auth_sessions(workspace_id);
	`)
	if err == nil {
		s.tableReady = true
	}
}

// CreateSession persists a new RFC 8628 CLI device session.
func (s *PostgresCLISessionStore) CreateSession(ctx context.Context, session *cliauth.CLIDeviceSession) error {
	// Always store in memory for guaranteed responsiveness
	_ = s.fallback.CreateSession(ctx, session)

	s.ensureTable(ctx)
	if s.repo != nil {
		// A DB write failure must never be silent. It used to be discarded
		// here, which is how a NOT NULL violation on session_id survived
		// unnoticed: device sessions never persisted, and the memory
		// fallback hid it until the process restarted or scaled out.
		// The caller still succeeds (memory has the session), so surface the
		// durability failure in logs rather than as a user-facing error.
		// AUDIT_REMEDIATION.md F-27.
		if err := s.repo.CreateCLISession(ctx, session); err != nil {
			log.Printf("[cli-session] WARN durable store unavailable, session %s is memory-only: %v",
				session.UUID, err)
		}
	}
	return nil
}

// GetByDeviceCode retrieves a session using its device_code.
func (s *PostgresCLISessionStore) GetByDeviceCode(ctx context.Context, deviceCode string) (*cliauth.CLIDeviceSession, error) {
	if s.repo != nil {
		if sess, err := s.repo.GetCLISessionByDeviceCode(ctx, deviceCode); err == nil && sess != nil {
			return sess, nil
		}
	}
	return s.fallback.GetByDeviceCode(ctx, deviceCode)
}

// GetByUserCode retrieves a pending session using its user_code.
func (s *PostgresCLISessionStore) GetByUserCode(ctx context.Context, userCode string) (*cliauth.CLIDeviceSession, error) {
	if s.repo != nil {
		if sess, err := s.repo.GetCLISessionByUserCode(ctx, userCode); err == nil && sess != nil {
			return sess, nil
		}
	}
	return s.fallback.GetByUserCode(ctx, userCode)
}

// CompleteSession updates the session status to completed and saves authorization tokens.
// CompleteSession records the approval. The workspace is the tenant that
// approved the device code in the browser; the CLI never supplies it.
// AUDIT_REMEDIATION.md F-37.
func (s *PostgresCLISessionStore) CompleteSession(ctx context.Context, userCode string, accessToken, refreshToken string, userID, workspaceID uuid.UUID, email string) error {
	// Fail closed: without a tenant the session must not be completed, because
	// the CLI would receive a token scoped to nothing.
	if workspaceID == uuid.Nil {
		return cliauth.ErrNoWorkspace
	}
	_ = s.fallback.CompleteSession(ctx, userCode, accessToken, refreshToken, userID, workspaceID, email)
	if s.repo != nil {
		_ = s.repo.CompleteCLISession(ctx, userCode, accessToken, refreshToken, userID, workspaceID, email)
	}
	return nil
}

// MarkConsumed updates the session status to consumed to prevent replay attacks.
func (s *PostgresCLISessionStore) MarkConsumed(ctx context.Context, sessionID uuid.UUID) error {
	_ = s.fallback.MarkConsumed(ctx, sessionID)
	if s.repo != nil {
		_ = s.repo.ConsumeCLISession(ctx, sessionID)
	}
	return nil
}

// ConsumeAndGetSession atomically claims a completed session for exactly one
// caller and returns it.
//
// AUDIT_REMEDIATION.md F-27. The database path is authoritative when the
// session has been persisted; the in-memory fallback is only consulted when
// there is no repository, and it is itself atomic under a write lock.
func (s *PostgresCLISessionStore) ConsumeAndGetSession(ctx context.Context, deviceCode string) (*cliauth.CLIDeviceSession, error) {
	if s.repo != nil {
		return s.repo.ConsumeCLISessionByDeviceCode(ctx, deviceCode)
	}
	return s.fallback.ConsumeAndGetSession(ctx, deviceCode)
}

// Compile-time check for interface implementation
var _ cliauth.SessionStore = (*PostgresCLISessionStore)(nil)
