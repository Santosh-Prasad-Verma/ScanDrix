package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashKeyDJB2AndFNV1a(t *testing.T) {
	key := "job:018f23a5-9b7b-7a54-8238-f407b8b29c91"
	hash := workflow.HashKey(key)

	assert.True(t, hash[0] >= 0, "hash1 should be non-negative")
	assert.True(t, hash[1] >= 0, "hash2 should be non-negative")

	// Same key produces identical hash
	hash2 := workflow.HashKey(key)
	assert.Equal(t, hash, hash2)

	// Different keys produce different hashes
	hashDiff := workflow.HashKey("job:different-key-12345")
	assert.NotEqual(t, hash, hashDiff)
}

func TestErrorClassifier(t *testing.T) {
	classifier := workflow.NewErrorClassifierService()

	// Retryable network/timeout
	assert.Equal(t, domain.ErrorClassificationRetryable, classifier.Classify(errors.New("i/o timeout on socket")))
	assert.Equal(t, domain.ErrorClassificationRetryable, classifier.Classify(errors.New("connection ECONNREFUSED")))
	assert.Equal(t, domain.ErrorClassificationRetryable, classifier.Classify(errors.New("ETIMEDOUT to api.github.com")))

	// Rate limited
	assert.Equal(t, domain.ErrorClassificationRateLimited, classifier.Classify(errors.New("GitHub API rate limit exceeded")))
	assert.Equal(t, domain.ErrorClassificationRateLimited, classifier.Classify(errors.New("HTTP 429 Too Many Requests")))

	// Non-retryable
	assert.Equal(t, domain.ErrorClassificationNonRetryable, classifier.Classify(errors.New("validation failed: missing owner")))
	assert.Equal(t, domain.ErrorClassificationNonRetryable, classifier.Classify(errors.New("repository not found")))
	assert.Equal(t, domain.ErrorClassificationNonRetryable, classifier.Classify(errors.New("unauthorized access to repository")))
}

type mockJobRepo struct {
	jobs    map[uuid.UUID]*workflow.WorkflowJobModel
	updates map[uuid.UUID]map[string]any
}

func (m *mockJobRepo) FindOne(ctx context.Context, id uuid.UUID) (*workflow.WorkflowJobModel, error) {
	return m.jobs[id], nil
}

func (m *mockJobRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	m.updates[id] = updates
	if job, ok := m.jobs[id]; ok {
		if s, ok := updates["status"].(domain.JobStatus); ok {
			job.Status = s
		}
	}
	return nil
}

type mockProcessor struct {
	processFn func(ctx context.Context, job *workflow.WorkflowJobModel) error
}

func (p *mockProcessor) Process(ctx context.Context, job *workflow.WorkflowJobModel) error {
	if p.processFn != nil {
		return p.processFn(ctx, job)
	}
	return nil
}

func TestJobProcessorRouterExecution(t *testing.T) {
	jobID := uuid.New()
	repo := &mockJobRepo{
		jobs: map[uuid.UUID]*workflow.WorkflowJobModel{
			jobID: {
				UUID:         jobID,
				WorkflowType: domain.WorkflowTypeCodeReview,
				Status:       domain.JobStatusPending,
			},
		},
		updates: make(map[uuid.UUID]map[string]any),
	}

	classifier := workflow.NewErrorClassifierService()
	router := workflow.NewJobProcessorRouterService(repo, classifier)

	executed := false
	router.Register(domain.WorkflowTypeCodeReview, &mockProcessor{
		processFn: func(ctx context.Context, job *workflow.WorkflowJobModel) error {
			executed = true
			return nil
		},
	})

	err := router.Process(context.Background(), jobID)
	require.NoError(t, err)
	assert.True(t, executed)
	assert.Equal(t, domain.JobStatusCompleted, repo.jobs[jobID].Status)
}

func TestJobProcessorRouterFailureHandling(t *testing.T) {
	jobID := uuid.New()
	repo := &mockJobRepo{
		jobs: map[uuid.UUID]*workflow.WorkflowJobModel{
			jobID: {
				UUID:         jobID,
				WorkflowType: domain.WorkflowTypeWebhookProcessing,
				Status:       domain.JobStatusPending,
			},
		},
		updates: make(map[uuid.UUID]map[string]any),
	}

	classifier := workflow.NewErrorClassifierService()
	router := workflow.NewJobProcessorRouterService(repo, classifier)

	router.Register(domain.WorkflowTypeWebhookProcessing, &mockProcessor{
		processFn: func(ctx context.Context, job *workflow.WorkflowJobModel) error {
			return errors.New("webhook validation failed: invalid signature")
		},
	})

	err := router.Process(context.Background(), jobID)
	assert.Error(t, err)
	assert.Equal(t, domain.JobStatusFailed, repo.jobs[jobID].Status)
	assert.Equal(t, domain.ErrorClassificationNonRetryable, repo.updates[jobID]["error_classification"])
}

func TestJobProcessorRouterPausedHandling(t *testing.T) {
	jobID := uuid.New()
	repo := &mockJobRepo{
		jobs: map[uuid.UUID]*workflow.WorkflowJobModel{
			jobID: {
				UUID:         jobID,
				WorkflowType: domain.WorkflowTypeCodeReview,
				Status:       domain.JobStatusPending,
			},
		},
		updates: make(map[uuid.UUID]map[string]any),
	}

	classifier := workflow.NewErrorClassifierService()
	router := workflow.NewJobProcessorRouterService(repo, classifier)

	router.Register(domain.WorkflowTypeCodeReview, &mockProcessor{
		processFn: func(ctx context.Context, job *workflow.WorkflowJobModel) error {
			return &workflow.WorkflowPausedError{
				EventType: "ast.task.completed",
				EventKey:  "repo:abc:123",
				StageName: "ASTAnalysisStage",
				TimeoutMs: 60000,
			}
		},
	})

	err := router.Process(context.Background(), jobID)
	assert.NoError(t, err)
	assert.Equal(t, domain.JobStatusWaitingForEvent, repo.jobs[jobID].Status)
	assert.Equal(t, domain.JobStatusWaitingForEvent, repo.updates[jobID]["status"])
	assert.Equal(t, "ASTAnalysisStage", repo.updates[jobID]["current_stage"])
	waitingForEvent, ok := repo.updates[jobID]["waiting_for_event"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "ast.task.completed", waitingForEvent["event_type"])
	assert.Equal(t, "repo:abc:123", waitingForEvent["event_key"])
}

type mockOutboxRepo struct {
	readyMessages []*workflow.OutboxMessageModel
	sentIDs       []uuid.UUID
	failedIDs     []uuid.UUID
}

func (m *mockOutboxRepo) FindReadyMessages(ctx context.Context, batchSize int) ([]*workflow.OutboxMessageModel, error) {
	return m.readyMessages, nil
}

func (m *mockOutboxRepo) MarkSent(ctx context.Context, id uuid.UUID) error {
	m.sentIDs = append(m.sentIDs, id)
	return nil
}

func (m *mockOutboxRepo) MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error {
	m.failedIDs = append(m.failedIDs, id)
	return nil
}

func (m *mockOutboxRepo) ScheduleRetry(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time, attempts int, lastError string) error {
	return nil
}

type mockPublisher struct {
	published []string
}

func (p *mockPublisher) Publish(ctx context.Context, exchange, routingKey string, payload map[string]any, options domain.BrokerPublishOptions) error {
	p.published = append(p.published, exchange+"/"+routingKey)
	return nil
}

func TestOutboxRelayBatch(t *testing.T) {
	msgID := uuid.New()
	outboxRepo := &mockOutboxRepo{
		readyMessages: []*workflow.OutboxMessageModel{
			{
				UUID:       msgID,
				Exchange:   "workflow.exchange",
				RoutingKey: "workflow.jobs.code_review.CODE_REVIEW",
				Payload:    map[string]any{"jobId": "123"},
				Status:     domain.OutboxStatusReady,
			},
		},
	}
	publisher := &mockPublisher{}

	relay := workflow.NewOutboxRelayService(outboxRepo, publisher, nil, nil, nil, nil)
	count, err := relay.RelayBatch(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Contains(t, publisher.published, "workflow.exchange/workflow.jobs.code_review.CODE_REVIEW")
	assert.Contains(t, outboxRepo.sentIDs, msgID)
}

func TestCalculateBackoffExponential(t *testing.T) {
	b1 := workflow.CalculateBackoff(1)
	b2 := workflow.CalculateBackoff(2)
	b3 := workflow.CalculateBackoff(3)

	assert.True(t, b2 > b1)
	assert.True(t, b3 > b2)
}
