package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/cron"
	"github.com/scandrix/backend/internal/queue/consumer"
)

// DrainManager coordinates graceful worker drain on shutdown signals.
type DrainManager struct {
	timeout time.Duration
}

// NewDrainManager creates a new drain manager with configurable timeout.
func NewDrainManager(timeoutMs int) *DrainManager {
	if timeoutMs <= 0 {
		timeoutMs = 25000
	}
	return &DrainManager{
		timeout: time.Duration(timeoutMs) * time.Millisecond,
	}
}

// Drain executes an orderly shutdown of cron schedulers and worker pools.
func (d *DrainManager) Drain(
	cancel context.CancelFunc,
	cronScheduler *cron.Scheduler,
	workerPool *consumer.WorkerPool,
) {
	slog.Info("Worker drain: stopping new intake and draining active jobs", "timeout", d.timeout)

	// Stop cron schedulers first so no new jobs are initiated
	if cronScheduler != nil {
		cronScheduler.Stop()
	}

	// Cancel context to stop consumer loops from fetching new messages
	if cancel != nil {
		cancel()
	}

	// Drain worker pool within the timeout window
	done := make(chan struct{})
	go func() {
		if workerPool != nil {
			workerPool.Stop()
		}
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Worker drain: all active tasks completed successfully")
	case <-time.After(d.timeout):
		slog.Warn("Worker drain: timeout exceeded, force terminating remaining tasks", "timeout", d.timeout)
	}
}
