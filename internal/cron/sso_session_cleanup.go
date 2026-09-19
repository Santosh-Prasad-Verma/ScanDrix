package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// TestSessionWorkbenchCleaner cleans expired transient SSO test sessions.
type TestSessionWorkbenchCleaner interface {
	CleanupExpired() int
}

// SSOSessionCleanup purges expired terminal device authorization states and transient SSO tokens.
type SSOSessionCleanup struct {
	repo             *database.Repository
	interval         time.Duration
	workbenchCleaner TestSessionWorkbenchCleaner
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

// SetWorkbenchCleaner attaches an optional workbench cleaner for ephemeral SSO test sessions.
func (s *SSOSessionCleanup) SetWorkbenchCleaner(cleaner TestSessionWorkbenchCleaner) {
	s.workbenchCleaner = cleaner
}

func (s *SSOSessionCleanup) Name() string {
	return "SSOSessionCleanup"
}

func (s *SSOSessionCleanup) Interval() time.Duration {
	return s.interval
}

func (s *SSOSessionCleanup) Run(ctx context.Context) error {
	if s.workbenchCleaner != nil {
		prunedWorkbench := s.workbenchCleaner.CleanupExpired()
		if prunedWorkbench > 0 {
			slog.Debug("Expired SSO test sessions cleaned", "pruned_count", prunedWorkbench)
		}
	}

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
