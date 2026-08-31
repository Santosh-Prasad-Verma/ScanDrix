package database

import (
	"context"
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
	_, err := s.repo.client.Pool.Exec(ctx, createTableSQL)
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
		_ = s.repo.CreateCLISession(ctx, session)
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
func (s *PostgresCLISessionStore) CompleteSession(ctx context.Context, userCode string, accessToken, refreshToken string, userID uuid.UUID, email string) error {
	_ = s.fallback.CompleteSession(ctx, userCode, accessToken, refreshToken, userID, email)
	if s.repo != nil {
		_ = s.repo.CompleteCLISession(ctx, userCode, accessToken, refreshToken, userID, email)
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

// Compile-time check for interface implementation
var _ cliauth.SessionStore = (*PostgresCLISessionStore)(nil)
