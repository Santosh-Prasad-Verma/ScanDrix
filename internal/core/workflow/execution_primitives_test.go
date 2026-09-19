package workflow

import (
	"context"
	"testing"
	"time"
)

func TestRunWithTimeoutSuccess(t *testing.T) {
	val, err := RunWithTimeout(context.Background(), 200*time.Millisecond, "timeout hit", func(ctx context.Context) (string, error) {
		return "success", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "success" {
		t.Fatalf("expected success, got %s", val)
	}
}

func TestRunWithTimeoutTriggered(t *testing.T) {
	_, err := RunWithTimeout(context.Background(), 50*time.Millisecond, "operation timed out", func(ctx context.Context) (string, error) {
		select {
		case <-time.After(150 * time.Millisecond):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestRaceWithAbortSignal(t *testing.T) {
	cancelChan := make(chan struct{})

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(cancelChan)
	}()

	_, err := RaceWithAbortSignal(cancelChan, func() (int, error) {
		time.Sleep(200 * time.Millisecond)
		return 100, nil
	})

	if err != ErrJobAborted {
		t.Fatalf("expected ErrJobAborted, got %v", err)
	}
}

func TestNoOpTaskProtection(t *testing.T) {
	svc := &NoopTaskProtectionService{}
	if err := svc.AcquireProtection(context.Background(), "task-1", 10*time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := svc.ReleaseProtection(context.Background(), "task-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueueArguments(t *testing.T) {
	args := GetQueueArguments("scandrix.workflow.jobs.code_review.queue")
	if args["x-queue-type"] != "quorum" {
		t.Fatalf("expected quorum queue type")
	}
}
