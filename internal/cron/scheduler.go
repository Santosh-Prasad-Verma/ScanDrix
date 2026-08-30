package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CronJob represents an individual background maintenance task.
type CronJob interface {
	Name() string
	Interval() time.Duration
	Run(ctx context.Context) error
}

// Scheduler orchestrates periodic background cron jobs with graceful shutdown and panic recovery.
type Scheduler struct {
	mu     sync.Mutex
	jobs   []CronJob
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewScheduler creates an initialized cron scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		jobs: make([]CronJob, 0),
	}
}

// Register adds a cron job to the scheduler schedule.
func (s *Scheduler) Register(job CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
}

// Start launches all registered cron job routines concurrently.
func (s *Scheduler) Start(parentCtx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(parentCtx)

	for _, job := range s.jobs {
		s.wg.Add(1)
		go s.runJobLoop(job)
	}

	slog.Info("Background cron scheduler started", "registered_jobs", len(s.jobs))
}

func (s *Scheduler) runJobLoop(job CronJob) {
	defer s.wg.Done()

	if job == nil {
		slog.Warn("Skipping nil cron job registration")
		return
	}

	interval := job.Interval()
	if interval <= 0 {
		slog.Warn("Skipping cron job with non-positive interval", "job", job.Name(), "interval", interval)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on startup
	s.executeSafely(job)

	for {
		select {
		case <-s.ctx.Done():
			slog.Debug("Cron job stopping on context cancellation", "job", job.Name())
			return
		case <-ticker.C:
			s.executeSafely(job)
		}
	}
}

func (s *Scheduler) executeSafely(job CronJob) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Cron job panicked", "job", job.Name(), "panic", fmt.Sprintf("%v", r))
		}
	}()

	start := time.Now()
	if err := job.Run(s.ctx); err != nil {
		if s.ctx.Err() != nil {
			return
		}
		slog.Warn("Cron job encountered an error", "job", job.Name(), "error", err, "duration", time.Since(start))
	} else {
		slog.Debug("Cron job completed successfully", "job", job.Name(), "duration", time.Since(start))
	}
}

// Stop gracefully signals all running cron jobs to terminate and waits for completion.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	s.wg.Wait()
	slog.Info("Background cron scheduler stopped cleanly")
}
