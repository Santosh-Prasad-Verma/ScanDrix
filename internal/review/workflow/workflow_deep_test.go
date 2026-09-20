package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInboxClaimEngine_LifecycleAndFencing(t *testing.T) {
	store := NewMemoryInboxStore()
	engine := NewInboxClaimEngine(store, 200*time.Millisecond)
	ctx := context.Background()

	msgID := "msg-review-123"
	worker1 := "worker-alpha"
	worker2 := "worker-beta"

	t.Run("Worker 1 Successfully Claims Task", func(t *testing.T) {
		claim, err := engine.TryClaim(ctx, msgID, worker1)
		require.NoError(t, err)
		assert.Equal(t, msgID, claim.MessageID)
		assert.Equal(t, worker1, claim.WorkerID)
		assert.Equal(t, ClaimStatusClaimed, claim.Status)
		assert.Equal(t, int64(1), claim.FencingToken)
	})

	t.Run("Worker 2 Rejected While Worker 1 Has Active Lease", func(t *testing.T) {
		claim, err := engine.TryClaim(ctx, msgID, worker2)
		assert.Error(t, err)
		assert.Nil(t, claim)
		assert.Contains(t, err.Error(), "actively leased")
	})

	t.Run("Worker 1 Renews Heartbeat", func(t *testing.T) {
		err := engine.RenewHeartbeat(ctx, msgID, worker1, 1, 300*time.Millisecond)
		require.NoError(t, err)
	})

	t.Run("Worker 2 Cannot Renew Worker 1 Lease (Fencing Check)", func(t *testing.T) {
		err := engine.RenewHeartbeat(ctx, msgID, worker2, 1, 300*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fencing error")
	})

	t.Run("Worker 1 Releases Claim as Completed", func(t *testing.T) {
		err := engine.ReleaseClaim(ctx, msgID, worker1, 1, ClaimStatusCompleted)
		require.NoError(t, err)

		// Idempotency check: Completed claim cannot be re-claimed
		_, err = engine.TryClaim(ctx, msgID, worker2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already completed")
	})
}

func TestInboxClaimEngine_LeaseExpirationAndReclaim(t *testing.T) {
	store := NewMemoryInboxStore()
	engine := NewInboxClaimEngine(store, 50*time.Millisecond)
	ctx := context.Background()

	msgID := "msg-abandoned-456"
	worker1 := "worker-crashed"
	worker2 := "worker-recovering"

	// Worker 1 claims but crashes
	claim1, err := engine.TryClaim(ctx, msgID, worker1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), claim1.FencingToken)

	// Wait for lease to expire
	time.Sleep(80 * time.Millisecond)

	// Worker 2 should now be able to reclaim with incremented fencing token
	claim2, err := engine.TryClaim(ctx, msgID, worker2)
	require.NoError(t, err)
	assert.Equal(t, worker2, claim2.WorkerID)
	assert.Greater(t, claim2.FencingToken, claim1.FencingToken)
	assert.Equal(t, 2, claim2.AttemptCount)
}

func TestDeadLetterEngine_ClassificationAndBackoff(t *testing.T) {
	dlq := NewDeadLetterEngine()
	ctx := context.Background()

	t.Run("Error Classification", func(t *testing.T) {
		assert.Equal(t, FailureRateLimited429, dlq.ClassifyError(errors.New("HTTP 429 Too Many Requests")))
		assert.Equal(t, FailureTransientNetwork, dlq.ClassifyError(errors.New("connection reset by peer")))
		assert.Equal(t, FailureSCMAuthExpired, dlq.ClassifyError(errors.New("GitHub API 401 Unauthorized")))
		assert.Equal(t, FailureFatalMalformedData, dlq.ClassifyError(errors.New("invalid json payload")))
		assert.Equal(t, FailureSandboxExhaustion, dlq.ClassifyError(errors.New("e2b cluster out of capacity")))
	})

	t.Run("Exponential Backoff with Jitter", func(t *testing.T) {
		d1 := dlq.CalculateBackoff(1)
		d2 := dlq.CalculateBackoff(2)
		d3 := dlq.CalculateBackoff(3)

		assert.Greater(t, d2, d1)
		assert.Greater(t, d3, d2)
		assert.LessOrEqual(t, d3, 35*time.Second) // Bounded by max delay
	})

	t.Run("Transient Failure Routes to Retry Queue", func(t *testing.T) {
		msgID := "msg-retry-1"
		err := errors.New("connection reset by peer")

		env := dlq.HandleFailure(ctx, msgID, "reviews.inbox", "review.start", []byte("payload"), 1, err)
		assert.False(t, env.IsDeadLettered)
		assert.False(t, env.NextRetryAt.IsZero())

		// Acknowledge after successful retry
		dlq.AcknowledgeSuccess(msgID)
		assert.Empty(t, dlq.GetReadyRetries(time.Now().Add(1*time.Hour)))
	})

	t.Run("Fatal Malformed Data Routes Immediately to DLQ", func(t *testing.T) {
		msgID := "msg-fatal-1"
		err := errors.New("invalid json syntax")

		env := dlq.HandleFailure(ctx, msgID, "reviews.inbox", "review.start", []byte("bad-json"), 1, err)
		assert.True(t, env.IsDeadLettered)
		assert.False(t, env.DeadLetteredAt.IsZero())

		dlqMsgs := dlq.GetDLQMessages()
		require.Len(t, dlqMsgs, 1)
		assert.Equal(t, msgID, dlqMsgs[0].MessageID)
	})
}

// dummyPipelineExecutor simulates pipeline execution for integration testing.
type dummyPipelineExecutor struct {
	err error
}

func (d *dummyPipelineExecutor) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	return d.err
}

func TestDistributedJobOrchestrator_ConcurrentProcessing(t *testing.T) {
	strategy := &dummyPipelineExecutor{err: nil}
	processor := NewReviewJobProcessor(strategy, nil, nil)

	claimStore := NewMemoryInboxStore()
	claimEngine := NewInboxClaimEngine(claimStore, 5*time.Second)
	dlqEngine := NewDeadLetterEngine()

	cfg := OrchestratorWorkerConfig{
		WorkerID:          "test-worker-cluster",
		WorkerCount:       4,
		HeartbeatInterval: 50 * time.Millisecond,
		LeaseDuration:     1 * time.Second,
		SweepInterval:     500 * time.Millisecond,
	}

	orch := NewDistributedJobOrchestrator(processor, claimEngine, dlqEngine, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := orch.Start(ctx)
	require.NoError(t, err)

	// Submit 10 concurrent review jobs
	var wg sync.WaitGroup
	jobCount := 10

	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := ReviewJobPayload{
				ReviewID:      uuid.New(),
				WorkspaceID:   uuid.New(),
				RepositoryID:  uuid.New(),
				RepoNamespace: "owner/repo",
				Provider:      models.ProviderGitHub,
				PullNumber:    100 + idx,
				Title:         fmt.Sprintf("PR #%d Fix Bug", 100+idx),
				HeadSHA:       fmt.Sprintf("sha-%d", idx),
				RawDiff:       "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
			}
			raw, _ := json.Marshal(payload)
			event := OrchestrationEvent{
				MessageID:  fmt.Sprintf("event-job-%d", idx),
				QueueName:  "reviews.inbox",
				RoutingKey: "review.process",
				Payload:    raw,
				ReceivedAt: time.Now().UTC(),
			}
			_ = orch.SubmitEvent(ctx, event)
		}(i)
	}

	wg.Wait()

	// Allow workers to process
	time.Sleep(200 * time.Millisecond)

	telemetry := orch.GetTelemetry()
	assert.True(t, telemetry["is_running"].(bool))
	assert.Equal(t, 4, telemetry["worker_count"])

	// Stop orchestrator gracefully
	orch.Stop()
}
