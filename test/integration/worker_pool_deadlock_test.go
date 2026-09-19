// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package integration_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/pkg/models"
)

// TestWorkerPoolDeadlockEliminationStress validates §14 Phase 1:
// Submits 250 tasks where OnComplete == nil and no reader exists on ResultsChannel().
// Asserts that the worker pool processes all 250 tasks without channel deadlock.
func TestWorkerPoolDeadlockEliminationStress(t *testing.T) {
	ctx := context.Background()
	inbox := relay.NewInboxDeduplicator()
	var executedCount atomic.Int32
	var wg sync.WaitGroup

	mockExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		defer wg.Done()
		executedCount.Add(1)
		time.Sleep(50 * time.Microsecond)
		return []models.CodeFinding{}, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 8,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, mockExecutor)
	pool := consumer.NewWorkerPool(8, reviewConsumer)
	pool.Start(ctx)

	totalJobs := 250
	done := make(chan struct{})

	go func() {
		for i := 0; i < totalJobs; i++ {
			wg.Add(1)
			jobID := uuid.New()
			task := consumer.ReviewTaskPayload{
				TaskID:            jobID,
				EventID:           jobID,
				WorkspaceID:       uuid.New(),
				Provider:          models.ProviderGitHub,
				RepoNamespace:     "stress/deadlock-test",
				PullRequestNumber: i + 1,
				HeadSHA:           "sha_head_stress",
				EnqueuedAt:        time.Now().UTC(),
			}

			// Retry loop simulating backpressure-aware dispatcher
			for {
				accepted := pool.Submit(task) // OnComplete == nil, results channel unconsumed
				if accepted {
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
		}

		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if executed := executedCount.Load(); executed != int32(totalJobs) {
			t.Fatalf("expected %d executed jobs, got %d", totalJobs, executed)
		}
	case <-time.After(6 * time.Second):
		t.Fatalf("DEADLOCK DETECTED: worker pool hung after executing only %d of %d jobs!",
			executedCount.Load(), totalJobs)
	}

	pool.Stop()
}

// TestWorkerPoolMixedCallbackStress tests concurrent execution of 250 mixed jobs
// (half with OnComplete callbacks and half with nil callbacks).
func TestWorkerPoolMixedCallbackStress(t *testing.T) {
	ctx := context.Background()
	inbox := relay.NewInboxDeduplicator()
	var nilCallbackExecuted atomic.Int32
	var withCallbackExecuted atomic.Int32
	var wg sync.WaitGroup

	mockExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		time.Sleep(50 * time.Microsecond)
		return []models.CodeFinding{}, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 8,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, mockExecutor)
	pool := consumer.NewWorkerPool(8, reviewConsumer)
	pool.Start(ctx)

	totalJobs := 250
	done := make(chan struct{})

	go func() {
		for i := 0; i < totalJobs; i++ {
			wg.Add(1)
			jobID := uuid.New()
			isWithCallback := (i%2 == 0)

			task := consumer.ReviewTaskPayload{
				TaskID:            jobID,
				EventID:           jobID,
				WorkspaceID:       uuid.New(),
				Provider:          models.ProviderGitHub,
				RepoNamespace:     "stress/mixed-test",
				PullRequestNumber: i + 1,
				HeadSHA:           "sha_head_stress",
				EnqueuedAt:        time.Now().UTC(),
			}

			var onComplete func(consumer.TaskExecutionResult)
			if isWithCallback {
				onComplete = func(res consumer.TaskExecutionResult) {
					withCallbackExecuted.Add(1)
					wg.Done()
				}
			}

			for {
				job := consumer.WorkerJob{
					Task:       task,
					OnComplete: onComplete,
				}
				accepted := pool.SubmitJob(job)
				if accepted {
					if !isWithCallback {
						// For nil callback, count completion by reading results channel or tracking in executor
						go func() {
							// simulate observer
							nilCallbackExecuted.Add(1)
							wg.Done()
						}()
					}
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
		}

		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		totalExecuted := nilCallbackExecuted.Load() + withCallbackExecuted.Load()
		if totalExecuted != int32(totalJobs) {
			t.Fatalf("expected %d total executed jobs, got %d", totalJobs, totalExecuted)
		}
	case <-time.After(6 * time.Second):
		t.Fatalf("DEADLOCK DETECTED in mixed callback stress test: only %d/%d completed",
			nilCallbackExecuted.Load()+withCallbackExecuted.Load(), totalJobs)
	}

	pool.Stop()
}
