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
	for res := range pool.ResultsChannel() {
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
