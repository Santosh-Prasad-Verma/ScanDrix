package cron

import (
	"context"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// ClassifyOrphanedSessionsCron sweeps abandoned or interrupted CLI and agent trace sessions,
// classifying them and releasing locked session resources.
type ClassifyOrphanedSessionsCron struct {
	repo              *database.Repository
	interval          time.Duration
	inactivityMinutes int
	batchLimit        int
}

// NewClassifyOrphanedSessionsCron creates a new orphaned session classifier cron job.
func NewClassifyOrphanedSessionsCron(repo *database.Repository, interval time.Duration, inactivityMinutes, batchLimit int) *ClassifyOrphanedSessionsCron {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	if inactivityMinutes <= 0 {
		inactivityMinutes = 30
	}
	if batchLimit <= 0 {
		batchLimit = 25
	}
	return &ClassifyOrphanedSessionsCron{
		repo:              repo,
		interval:          interval,
		inactivityMinutes: inactivityMinutes,
		batchLimit:        batchLimit,
	}
}

func (c *ClassifyOrphanedSessionsCron) Name() string {
	return "ClassifyOrphanedSessionsCron"
}

func (c *ClassifyOrphanedSessionsCron) Interval() time.Duration {
	return c.interval
}

func (c *ClassifyOrphanedSessionsCron) Run(ctx context.Context) error {
	if c.repo == nil {
		return nil
	}

	start := time.Now()
	orphaned, err := c.repo.FindOrphanedCLISessions(ctx, c.inactivityMinutes, c.batchLimit)
	if err != nil {
		slog.Error("Failed querying orphaned CLI sessions", "error", err)
		return err
	}

	if len(orphaned) == 0 {
		return nil
	}

	slog.Info("Classifying orphaned CLI sessions", "count", len(orphaned))

	classifiedCount := 0
	for _, s := range orphaned {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		classification := "TIMEOUT_INACTIVE"
		if s.Status == "authorized" {
			classification = "DISCONNECTED_AUTHORIZED"
		}

		if err := c.repo.MarkCLISessionClassified(ctx, s.ID, classification); err != nil {
			slog.Warn("Failed classifying orphaned session", "session_id", s.SessionID, "error", err)
			continue
		}
		classifiedCount++
	}

	slog.Info("Completed orphaned session classification",
		"classified", classifiedCount,
		"total_found", len(orphaned),
		"duration", time.Since(start),
	)
	return nil
}
