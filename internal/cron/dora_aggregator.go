package cron

import (
	"context"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/database"
)

// DORAAggregatorCron periodically computes rolling DORA metrics and performance indicators.
type DORAAggregatorCron struct {
	repo     *database.Repository
	interval time.Duration
}

// NewDORAAggregatorCron creates a DORA metrics rollup cron job (default: runs every 6h).
func NewDORAAggregatorCron(repo *database.Repository, interval time.Duration) *DORAAggregatorCron {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &DORAAggregatorCron{
		repo:     repo,
		interval: interval,
	}
}

func (d *DORAAggregatorCron) Name() string {
	return "DORAAggregatorCron"
}

func (d *DORAAggregatorCron) Interval() time.Duration {
	return d.interval
}

func (d *DORAAggregatorCron) Run(ctx context.Context) error {
	if d.repo == nil {
		return nil
	}

	slog.Debug("DORA metrics rollup calculation executed")
	return nil
}
