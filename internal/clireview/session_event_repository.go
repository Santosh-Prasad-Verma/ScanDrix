package clireview

import (
	"github.com/scandrix/backend/internal/clireview/infrastructure/repositories"
)

// SessionEventRecord represents a fully persisted session event with classification state.
type SessionEventRecord = repositories.SessionEventRecord

// SessionEventRepository provides thread-safe in-memory and relational storage for telemetry events.
type SessionEventRepository = repositories.SessionEventRepository

// NewSessionEventRepository creates an initialized SessionEventRepository.
func NewSessionEventRepository() *SessionEventRepository {
	return repositories.NewSessionEventRepository()
}
