package clireview

import (
	"github.com/scandrix/backend/internal/clireview/infrastructure/repositories"
)

// CliSessionCaptureRepository provides thread-safe storage for CLI session captures.
type CliSessionCaptureRepository = repositories.CliSessionCaptureRepository

// NewCliSessionCaptureRepository initializes the repository.
func NewCliSessionCaptureRepository() *CliSessionCaptureRepository {
	return repositories.NewCliSessionCaptureRepository()
}
