package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// LicenseSeatPruner reclaims license seats from inactive organization members.
type LicenseSeatPruner struct {
	repo           *database.Repository
	interval       time.Duration
	inactivityDays int
}

// NewLicenseSeatPruner creates an inactive seat pruner (default: checks every 24h for members inactive > 30 days).
func NewLicenseSeatPruner(repo *database.Repository, interval time.Duration, inactivityDays int) *LicenseSeatPruner {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if inactivityDays <= 0 {
		inactivityDays = 30
	}
	return &LicenseSeatPruner{
		repo:           repo,
		interval:       interval,
		inactivityDays: inactivityDays,
	}
}

func (p *LicenseSeatPruner) Name() string {
	return "LicenseSeatPruner"
}

func (p *LicenseSeatPruner) Interval() time.Duration {
	return p.interval
}

func (p *LicenseSeatPruner) Run(ctx context.Context) error {
	if p.repo == nil {
		return nil
	}

	pruned, err := p.repo.PruneInactiveLicenseSeats(ctx, p.inactivityDays)
	if err != nil {
		return fmt.Errorf("license seat pruner failed: %w", err)
	}

	if pruned > 0 {
		slog.Info("Inactive license seats pruned and reclaimed", "pruned_count", pruned, "inactivity_days", p.inactivityDays)
	}
	return nil
}
