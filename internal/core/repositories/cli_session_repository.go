package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// CliSessionRepository defines operations for interactive CLI browser login handshakes.
//
// The Postgres implementation that used to live here has been removed. It had
// zero callers and its SQL referenced four columns that do not exist in
// cli_auth_sessions (session_code, token_payload, client_ip, updated_at), so
// every statement it could issue was guaranteed to fail. Leaving it beside a
// working path invited someone to wire it up.
//
// The live device-flow path is PostgresCLISessionStore in internal/database,
// which is wired at cmd/api/main.go. This interface is retained because
// MockCliSessionRepo implements it and is exercised by the lifecycle tests.
//
// AUDIT_REMEDIATION.md F-16 / F-37.
type CliSessionRepository interface {
	Create(ctx context.Context, session *domain.CliAuthSession) error
	FindBySessionCode(ctx context.Context, sessionCode string) (*domain.CliAuthSession, error)
	FindByUserCode(ctx context.Context, userCode string) (*domain.CliAuthSession, error)
	Authorize(ctx context.Context, userCode string, userID, wsID uuid.UUID, tokenPayload string) error
	Complete(ctx context.Context, sessionCode string) error
	PurgeExpired(ctx context.Context) (int64, error)
}
