package consumer_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/pkg/models"
)

func TestReviewConsumerAndWorkerPool(t *testing.T) {
	ctx := context.Background()
	inbox := relay.NewInboxDeduplicator()

	var successCounter int64
	mockSuccessExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		atomic.AddInt64(&successCounter, 1)
		return []models.CodeFinding{
			{Title: "Test Finding", Severity: models.SeverityLow},
		}, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 4,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, mockSuccessExecutor)

	// 1. Process Valid Task -> Success
	task1 := consumer.ReviewTaskPayload{
		TaskID:            uuid.New(),
		WorkspaceID:       uuid.New(),
		Provider:          models.ProviderGitHub,
		RepoNamespace:     "acme/core",
		PullRequestNumber: 101,
		HeadSHA:           "sha_head_101",
		EnqueuedAt:        time.Now().UTC(),
	}

	res1 := reviewConsumer.ProcessTask(ctx, task1)
	if res1.Status != consumer.TaskStatusSuccess {
		t.Fatalf("expected SUCCESS, got %s: %s", res1.Status, res1.ErrorMsg)
	}

	// 2. Inbox Deduplication: Submitting task1 again must be detected as duplicate
	resDuplicate := reviewConsumer.ProcessTask(ctx, task1)
	if resDuplicate.Status != consumer.TaskStatusDuplicate {
		t.Fatalf("expected DUPLICATE on re-processing, got %s", resDuplicate.Status)
	}

	// 3. Retry and Dead Letter Queue (DLQ)
	failingExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		return nil, errors.New("upstream git clone timeout")
	}

	failingInbox := relay.NewInboxDeduplicator()
	failingConsumer := consumer.NewReviewConsumer(cfg, failingInbox, failingExecutor)

	failTask := consumer.ReviewTaskPayload{
		TaskID:            uuid.New(),
		WorkspaceID:       uuid.New(),
		Provider:          models.ProviderGitLab,
		RepoNamespace:     "acme/api",
		PullRequestNumber: 42,
		AttemptCount:      0,
	}

	// Attempt 1: RETRY
	r1 := failingConsumer.ProcessTask(ctx, failTask)
	if r1.Status != consumer.TaskStatusRetry {
		t.Fatalf("expected RETRY on attempt 1, got %s", r1.Status)
	}

	// Attempt 2: RETRY
	failTask.AttemptCount = 1
	r2 := failingConsumer.ProcessTask(ctx, failTask)
	if r2.Status != consumer.TaskStatusRetry {
		t.Fatalf("expected RETRY on attempt 2, got %s", r2.Status)
	}

	// Attempt 3: Exhausted -> DEAD_LETTER
	failTask.AttemptCount = 2
	r3 := failingConsumer.ProcessTask(ctx, failTask)
	if r3.Status != consumer.TaskStatusDeadLetter {
		t.Fatalf("expected DEAD_LETTER on attempt 3, got %s", r3.Status)
	}

	dlq := failingConsumer.GetDeadLetters()
	if len(dlq) != 1 || dlq[0].TaskID != failTask.TaskID {
		t.Fatalf("expected 1 dead-lettered message, got %+v", dlq)
	}

	// 4. Concurrent WorkerPool Execution
	pool := consumer.NewWorkerPool(4, reviewConsumer)

	// Subscribe before starting: results produced with no registered reader are
	// discarded, so reading only after submission can silently lose them.
	resultsCh := pool.ResultsChannel()
	pool.Start(ctx)

	for i := 0; i < 8; i++ {
		pool.Submit(consumer.ReviewTaskPayload{
			TaskID:            uuid.New(),
			WorkspaceID:       uuid.New(),
			Provider:          models.ProviderGitHub,
			RepoNamespace:     "acme/worker-test",
			PullRequestNumber: i + 1,
		})
	}

	// Read 8 results
	processed := 0
	for res := range resultsCh {
		if res.Status != consumer.TaskStatusSuccess {
			t.Fatalf("worker pool execution failed on task: %+v", res)
		}
		processed++
		if processed == 8 {
			break
		}
	}

	pool.Stop()
	if processed != 8 {
		t.Fatalf("expected 8 processed tasks, got %d", processed)
	}
}

func TestTaskZeroAttemptReachesRetryLimit(t *testing.T) {
	ctx := context.Background()
	inbox := relay.NewInboxDeduplicator()

	failingExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		return nil, errors.New("temporary upstream service failure")
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 2,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, failingExecutor)
	taskID := uuid.New()

	// Initial task payload arrives with AttemptCount: 0
	taskPayload := consumer.ReviewTaskPayload{
		TaskID:            taskID,
		WorkspaceID:       uuid.New(),
		Provider:          models.ProviderGitHub,
		RepoNamespace:     "acme/retry-test",
		PullRequestNumber: 99,
		AttemptCount:      0,
	}

	// Attempt 1: should return RETRY with AttemptCount: 1
	r1 := reviewConsumer.ProcessTask(ctx, taskPayload)
	if r1.Status != consumer.TaskStatusRetry || r1.AttemptCount != 1 {
		t.Fatalf("expected RETRY with attempt 1, got status=%s attempt=%d", r1.Status, r1.AttemptCount)
	}

	// Attempt 2: Even if raw payload is redelivered without prior mutation, inbox tracks attempt count
	r2 := reviewConsumer.ProcessTask(ctx, taskPayload)
	if r2.Status != consumer.TaskStatusRetry || r2.AttemptCount != 2 {
		t.Fatalf("expected RETRY with attempt 2, got status=%s attempt=%d", r2.Status, r2.AttemptCount)
	}

	// Attempt 3: MaxRetries (3) reached -> DEAD_LETTER
	r3 := reviewConsumer.ProcessTask(ctx, taskPayload)
	if r3.Status != consumer.TaskStatusDeadLetter || r3.AttemptCount != 3 {
		t.Fatalf("expected DEAD_LETTER with attempt 3, got status=%s attempt=%d", r3.Status, r3.AttemptCount)
	}

	deadLetters := reviewConsumer.GetDeadLetters()
	if len(deadLetters) != 1 || deadLetters[0].TaskID != taskID {
		t.Fatalf("expected task %s in DLQ, got %+v", taskID, deadLetters)
	}
}

func TestWorkerPoolGracefulShutdown(t *testing.T) {
	ctx := context.Background()
	inbox := relay.NewInboxDeduplicator()

	var completedCounter int64
	var callbackCounter int64

	slowExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&completedCounter, 1)
		return nil, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 3,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, slowExecutor)
	pool := consumer.NewWorkerPool(3, reviewConsumer)
	pool.Start(ctx)

	const taskCount = 12
	for i := 0; i < taskCount; i++ {
		task := consumer.ReviewTaskPayload{
			TaskID:            uuid.New(),
			WorkspaceID:       uuid.New(),
			Provider:          models.ProviderGitHub,
			RepoNamespace:     "acme/shutdown-test",
			PullRequestNumber: i + 1,
		}
		pool.SubmitJob(consumer.WorkerJob{
			Task: task,
			OnComplete: func(res consumer.TaskExecutionResult) {
				if res.Status == consumer.TaskStatusSuccess {
					atomic.AddInt64(&callbackCounter, 1)
				}
			},
		})
	}

	// Stop must wait for all in-flight and queued jobs to complete
	pool.Stop()

	if atomic.LoadInt64(&completedCounter) != taskCount {
		t.Fatalf("expected %d tasks to complete execution on pool.Stop(), got %d", taskCount, completedCounter)
	}
	if atomic.LoadInt64(&callbackCounter) != taskCount {
		t.Fatalf("expected %d OnComplete callbacks to fire, got %d", taskCount, callbackCounter)
	}
}

func TestWorkerPoolUnconsumedResultsChannelNoDeadlock(t *testing.T) {
	ctx := context.Background()
	inbox := relay.NewInboxDeduplicator()

	var completedCounter int64
	fastExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		atomic.AddInt64(&completedCounter, 1)
		return nil, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 4,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, fastExecutor)
	pool := consumer.NewWorkerPool(4, reviewConsumer)
	pool.Start(ctx)

	// Submit 150 tasks (capacity of p.results is 100) using Submit (OnComplete == nil)
	// without any consumer reading from ResultsChannel(). Must not deadlock.
	const taskCount = 150
	for i := 0; i < taskCount; i++ {
		for !pool.Submit(consumer.ReviewTaskPayload{
			TaskID:            uuid.New(),
			WorkspaceID:       uuid.New(),
			Provider:          models.ProviderGitHub,
			RepoNamespace:     "acme/deadlock-test",
			PullRequestNumber: i + 1,
		}) {
			time.Sleep(time.Millisecond)
		}
	}

	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		if atomic.LoadInt64(&completedCounter) != taskCount {
			t.Fatalf("expected %d completed tasks, got %d", taskCount, completedCounter)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker pool deadlocked when results channel was unconsumed")
	}
}

// TestWorkerPoolResultsDeliveredBeforeStartRegression guards the result-delivery
// contract: a caller that registers a reader before Start must receive exactly one
// result per submitted job, even when the job count exceeds the results buffer and
// depends on the drain goroutine being scheduled first.
//
// This regressed as a flaky 30s timeout in CI: registering the reader from a
// goroutine after Start let the pool discard results produced in the gap, so the
// number lost varied with machine load.
func TestWorkerPoolResultsDeliveredBeforeStartRegression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	inbox := relay.NewInboxDeduplicator()
	var processed int64
	exec := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		atomic.AddInt64(&processed, 1)
		return nil, nil
	}

	pool := consumer.NewWorkerPool(8, consumer.NewReviewConsumer(
		consumer.ConsumerConfig{MaxRetries: 3, Concurrency: 8}, inbox, exec))

	// Subscribe BEFORE Start: this is the ordering the pool documents.
	resultsCh := pool.ResultsChannel()
	pool.Start(ctx)

	// Deliberately larger than the 100-slot results buffer, so delivery depends on
	// the drain goroutine running concurrently with submission.
	const totalJobs = 200
	var received int64
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range resultsCh {
			if atomic.AddInt64(&received, 1) >= totalJobs {
				return
			}
		}
	}()

	for i := 0; i < totalJobs; i++ {
		task := consumer.ReviewTaskPayload{
			TaskID:            uuid.New(),
			WorkspaceID:       uuid.New(),
			Provider:          models.ProviderGitHub,
			RepoNamespace:     "acme/delivery-regression",
			PullRequestNumber: i + 1,
		}
		for !pool.Submit(task) {
			select {
			case <-ctx.Done():
				t.Fatalf("timed out submitting job %d", i)
			default:
				time.Sleep(100 * time.Microsecond)
			}
		}
	}

	select {
	case <-drained:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for results: received %d/%d", atomic.LoadInt64(&received), totalJobs)
	}

	pool.Stop()

	if got := atomic.LoadInt64(&received); got != totalJobs {
		t.Fatalf("expected %d results delivered, got %d (%d discarded)", totalJobs, got, totalJobs-got)
	}
	if got := atomic.LoadInt64(&processed); got != totalJobs {
		t.Fatalf("expected executor to run %d jobs, got %d", totalJobs, got)
	}
}
