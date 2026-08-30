package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/cliauth"
)

// PostgresCLISessionStore adapts the PostgreSQL Repository to the cliauth.SessionStore interface.
type PostgresCLISessionStore struct {
	repo *Repository
}

// NewPostgresCLISessionStore initializes a new PostgreSQL-backed CLI session store.
func NewPostgresCLISessionStore(repo *Repository) *PostgresCLISessionStore {
	return &PostgresCLISessionStore{repo: repo}
}

// CreateSession persists a new RFC 8628 CLI device session to PostgreSQL.
func (s *PostgresCLISessionStore) CreateSession(ctx context.Context, session *cliauth.CLIDeviceSession) error {
	return s.repo.CreateCLISession(ctx, session)
}

// GetByDeviceCode retrieves a session using its device_code.
func (s *PostgresCLISessionStore) GetByDeviceCode(ctx context.Context, deviceCode string) (*cliauth.CLIDeviceSession, error) {
	return s.repo.GetCLISessionByDeviceCode(ctx, deviceCode)
}

// GetByUserCode retrieves a pending session using its user_code.
func (s *PostgresCLISessionStore) GetByUserCode(ctx context.Context, userCode string) (*cliauth.CLIDeviceSession, error) {
	return s.repo.GetCLISessionByUserCode(ctx, userCode)
}

// CompleteSession updates the session status to completed and saves authorization tokens.
func (s *PostgresCLISessionStore) CompleteSession(ctx context.Context, userCode string, accessToken, refreshToken string, userID uuid.UUID, email string) error {
	return s.repo.CompleteCLISession(ctx, userCode, accessToken, refreshToken, userID, email)
}

// MarkConsumed updates the session status to consumed to prevent replay attacks.
func (s *PostgresCLISessionStore) MarkConsumed(ctx context.Context, sessionID uuid.UUID) error {
	return s.repo.ConsumeCLISession(ctx, sessionID)
}

// Compile-time check for interface implementation
var _ cliauth.SessionStore = (*PostgresCLISessionStore)(nil)
