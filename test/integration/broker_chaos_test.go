// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package integration_test

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

// TestBrokerChaosAndDisconnectionRecovery simulates broker failure, disconnect chaos,
// worker channel disruption, and graceful recovery.
func TestBrokerChaosAndDisconnectionRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inbox := relay.NewInboxDeduplicator()
	var executed atomic.Int32
	var transientFails atomic.Int32

	// Flaky executor: simulates downstream broker / network disconnection
	chaosExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		count := executed.Add(1)
		// First 2 executions experience simulated network/broker disconnect
		if count <= 2 {
			transientFails.Add(1)
			return nil, errors.New("simulated broker connection reset by peer")
		}
		return []models.CodeFinding{}, nil
	}

	cfg := consumer.ConsumerConfig{
		MaxRetries:  3,
		ClaimTTL:    1 * time.Minute,
		Concurrency: 4,
	}

	reviewConsumer := consumer.NewReviewConsumer(cfg, inbox, chaosExecutor)
	pool := consumer.NewWorkerPool(4, reviewConsumer)
	pool.Start(ctx)
	defer pool.Stop()

	// 1. Submit task that fails transiently
	taskID := uuid.New()
	task := consumer.ReviewTaskPayload{
		TaskID:            taskID,
		EventID:           taskID,
		WorkspaceID:       uuid.New(),
		Provider:          models.ProviderGitHub,
		RepoNamespace:     "chaos/broker-test",
		PullRequestNumber: 1,
		HeadSHA:           "sha_head_chaos",
		AttemptCount:      0,
		EnqueuedAt:        time.Now().UTC(),
	}

	completed := make(chan consumer.TaskExecutionResult, 10)
	pool.SubmitJob(consumer.WorkerJob{
		Task: task,
		OnComplete: func(res consumer.TaskExecutionResult) {
			completed <- res
		},
	})

	select {
	case res := <-completed:
		if res.Status != consumer.TaskStatusRetry {
			t.Fatalf("expected TaskStatusRetry on first failure, got %s", res.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial failure response")
	}

	// 2. Retry task with incremented attempt
	task.AttemptCount = 1
	pool.SubmitJob(consumer.WorkerJob{
		Task: task,
		OnComplete: func(res consumer.TaskExecutionResult) {
			completed <- res
		},
	})

	select {
	case res := <-completed:
		if res.Status != consumer.TaskStatusRetry {
			t.Fatalf("expected TaskStatusRetry on second failure, got %s", res.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second failure response")
	}

	// 3. Broker recovers: 3rd attempt succeeds
	task.AttemptCount = 2
	pool.SubmitJob(consumer.WorkerJob{
		Task: task,
		OnComplete: func(res consumer.TaskExecutionResult) {
			completed <- res
		},
	})

	select {
	case res := <-completed:
		if res.Status != consumer.TaskStatusSuccess {
			t.Fatalf("expected TaskStatusSuccess after recovery, got %s: %s", res.Status, res.ErrorMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered execution response")
	}

	// 4. Poison message: persistent failure moves to Dead Letter Queue (DLQ)
	poisonTaskID := uuid.New()
	poisonTask := consumer.ReviewTaskPayload{
		TaskID:            poisonTaskID,
		EventID:           poisonTaskID,
		WorkspaceID:       uuid.New(),
		Provider:          models.ProviderGitLab,
		RepoNamespace:     "chaos/poison-test",
		PullRequestNumber: 99,
		AttemptCount:      2, // Max retries is 3 (attempts 0, 1, 2)
		EnqueuedAt:        time.Now().UTC(),
	}

	failingInbox := relay.NewInboxDeduplicator()
	poisonConsumer := consumer.NewReviewConsumer(cfg, failingInbox, func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		return nil, errors.New("unrecoverable corrupted AST payload")
	})

	resDLQ := poisonConsumer.ProcessTask(ctx, poisonTask)
	if resDLQ.Status != consumer.TaskStatusDeadLetter {
		t.Fatalf("expected TaskStatusDeadLetter on max retry exhaustion, got %s", resDLQ.Status)
	}

	deadLetters := poisonConsumer.GetDeadLetters()
	if len(deadLetters) != 1 || deadLetters[0].TaskID != poisonTaskID {
		t.Fatalf("expected poison message recorded in DLQ, got %+v", deadLetters)
	}
}
