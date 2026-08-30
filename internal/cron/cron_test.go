package cron_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cron"
)

type mockJob struct {
	name     string
	interval time.Duration
	runCount int64
	fail     bool
	panicErr bool
}

func (m *mockJob) Name() string {
	return m.name
}

func (m *mockJob) Interval() time.Duration {
	return m.interval
}

func (m *mockJob) Run(ctx context.Context) error {
	atomic.AddInt64(&m.runCount, 1)
	if m.panicErr {
		panic("simulated cron panic")
	}
	if m.fail {
		return errors.New("simulated cron error")
	}
	return nil
}

func TestSchedulerLifecycleAndExecution(t *testing.T) {
	scheduler := cron.NewScheduler()

	job1 := &mockJob{name: "Job1", interval: 20 * time.Millisecond}
	job2 := &mockJob{name: "Job2", interval: 30 * time.Millisecond}
	panicJob := &mockJob{name: "PanicJob", interval: 25 * time.Millisecond, panicErr: true}
	errorJob := &mockJob{name: "ErrorJob", interval: 25 * time.Millisecond, fail: true}

	scheduler.Register(job1)
	scheduler.Register(job2)
	scheduler.Register(panicJob)
	scheduler.Register(errorJob)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	scheduler.Start(ctx)

	// Wait for a few iterations
	time.Sleep(70 * time.Millisecond)

	scheduler.Stop()

	if atomic.LoadInt64(&job1.runCount) < 2 {
		t.Fatalf("expected job1 to run at least 2 times, got %d", atomic.LoadInt64(&job1.runCount))
	}
	if atomic.LoadInt64(&job2.runCount) < 1 {
		t.Fatalf("expected job2 to run at least 1 time, got %d", atomic.LoadInt64(&job2.runCount))
	}
	if atomic.LoadInt64(&panicJob.runCount) < 1 {
		t.Fatalf("expected panicJob to attempt execution, got %d", atomic.LoadInt64(&panicJob.runCount))
	}
	if atomic.LoadInt64(&errorJob.runCount) < 1 {
		t.Fatalf("expected errorJob to attempt execution, got %d", atomic.LoadInt64(&errorJob.runCount))
	}
}

func TestBuiltInWatchdogJobsWithNilRepo(t *testing.T) {
	ctx := context.Background()

	watchdog := cron.NewStaleReviewWatchdog(nil, time.Minute, 15)
	if watchdog.Name() != "StaleReviewWatchdog" {
		t.Fatalf("expected StaleReviewWatchdog name, got %s", watchdog.Name())
	}
	if err := watchdog.Run(ctx); err != nil {
		t.Fatalf("expected nil error on nil repo, got %v", err)
	}

	pruner := cron.NewLicenseSeatPruner(nil, time.Hour, 30)
	if pruner.Name() != "LicenseSeatPruner" {
		t.Fatalf("expected LicenseSeatPruner name, got %s", pruner.Name())
	}
	if err := pruner.Run(ctx); err != nil {
		t.Fatalf("expected nil error on nil repo, got %v", err)
	}

	ssoCleaner := cron.NewSSOSessionCleanup(nil, time.Hour)
	if ssoCleaner.Name() != "SSOSessionCleanup" {
		t.Fatalf("expected SSOSessionCleanup name, got %s", ssoCleaner.Name())
	}
	if err := ssoCleaner.Run(ctx); err != nil {
		t.Fatalf("expected nil error on nil repo, got %v", err)
	}

	doraCron := cron.NewDORAAggregatorCron(nil, time.Hour)
	if doraCron.Name() != "DORAAggregatorCron" {
		t.Fatalf("expected DORAAggregatorCron name, got %s", doraCron.Name())
	}
	if err := doraCron.Run(ctx); err != nil {
		t.Fatalf("expected nil error on nil repo, got %v", err)
	}
}

