package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// SSOSessionCleanup purges expired terminal device authorization states and transient SSO tokens.
type SSOSessionCleanup struct {
	repo     *database.Repository
	interval time.Duration
}

// NewSSOSessionCleanup initializes the SSO session cleaner (default: runs every 1h).
func NewSSOSessionCleanup(repo *database.Repository, interval time.Duration) *SSOSessionCleanup {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &SSOSessionCleanup{
		repo:     repo,
		interval: interval,
	}
}

func (s *SSOSessionCleanup) Name() string {
	return "SSOSessionCleanup"
}

func (s *SSOSessionCleanup) Interval() time.Duration {
	return s.interval
}

func (s *SSOSessionCleanup) Run(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}

	pruned, err := s.repo.PruneExpiredCLISessions(ctx)
	if err != nil {
		return fmt.Errorf("SSO session cleanup failed: %w", err)
	}

	if pruned > 0 {
		slog.Debug("Expired CLI and SSO device sessions cleaned", "pruned_count", pruned)
	}
	return nil
}
