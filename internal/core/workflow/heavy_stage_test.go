package workflow_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBufferServiceLifecycle(t *testing.T) {
	buffer := workflow.NewEventBufferService()

	event := workflow.StageCompletedEvent{
		EventType: "ast.task.completed",
		EventKey:  "repo:123:sha:abc",
		TaskID:    "task_xyz",
		Result:    map[string]any{"nodes": 450},
	}

	// 1. Store
	buffer.Store(event.EventType, event.EventKey, event, 500*time.Millisecond)

	// 2. Check (and consume)
	retrieved, found := buffer.Check(event.EventType, event.EventKey)
	assert.True(t, found)
	require.NotNil(t, retrieved)
	assert.Equal(t, "task_xyz", retrieved.TaskID)

	// 3. Second check should return not found (consumed)
	_, foundSecond := buffer.Check(event.EventType, event.EventKey)
	assert.False(t, foundSecond)

	// 4. Expiration
	buffer.Store(event.EventType, event.EventKey, event, 20*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	_, foundExpired := buffer.Check(event.EventType, event.EventKey)
	assert.False(t, foundExpired)
}

type mockWaitingJobRepo struct {
	mockJobRepo
	waitingJobs map[string][]*workflow.WorkflowJobModel
}

func (m *mockWaitingJobRepo) FindWaitingJobsByEvent(ctx context.Context, eventType, eventKey string) ([]*workflow.WorkflowJobModel, error) {
	key := eventType + ":" + eventKey
	return m.waitingJobs[key], nil
}

type mockInboxRepo struct {
	mu                sync.Mutex
	claimedMessages   map[string]bool
	completedMessages map[string]bool
	releasedMessages  map[string]bool
}

func newMockInboxRepo() *mockInboxRepo {
	return &mockInboxRepo{
		claimedMessages:   make(map[string]bool),
		completedMessages: make(map[string]bool),
		releasedMessages:  make(map[string]bool),
	}
}

func (m *mockInboxRepo) Claim(ctx context.Context, messageID, consumerID string, jobID *uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimedMessages[messageID] {
		return false, nil
	}
	m.claimedMessages[messageID] = true
	return true, nil
}

func (m *mockInboxRepo) Complete(ctx context.Context, messageID, consumerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedMessages[messageID] = true
	return nil
}

func (m *mockInboxRepo) Release(ctx context.Context, messageID, consumerID string, lastError error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releasedMessages[messageID] = true
	return nil
}

func TestHeavyStageEventHandlerResumption(t *testing.T) {
	jobID := uuid.New()
	waitingRepo := &mockWaitingJobRepo{
		mockJobRepo: mockJobRepo{
			jobs: map[uuid.UUID]*workflow.WorkflowJobModel{
				jobID: {
					UUID:         jobID,
					WorkflowType: domain.WorkflowTypeCodeReview,
					Status:       domain.JobStatusWaitingForEvent,
				},
			},
			updates: make(map[uuid.UUID]map[string]any),
		},
		waitingJobs: make(map[string][]*workflow.WorkflowJobModel),
	}

	stateMgr := workflow.NewPipelineStateManager(waitingRepo)
	eventBuffer := workflow.NewEventBufferService()
	inboxRepo := newMockInboxRepo()
	publisher := &mockPublisher{}

	handler := workflow.NewHeavyStageEventHandler(
		waitingRepo,
		stateMgr,
		eventBuffer,
		inboxRepo,
		publisher,
	)

	// Set job waiting for event
	eventType := "ast.task.completed"
	eventKey := "repo_test_123"
	waitingRepo.waitingJobs[eventType+":"+eventKey] = []*workflow.WorkflowJobModel{waitingRepo.jobs[jobID]}

	event := workflow.StageCompletedEvent{
		EventType: eventType,
		EventKey:  eventKey,
		TaskID:    "task_abc",
		Result:    map[string]any{"symbols": 120},
	}

	err := handler.OnStageCompleted(context.Background(), event)
	require.NoError(t, err)

	assert.Equal(t, domain.JobStatusProcessing, waitingRepo.jobs[jobID].Status)
	assert.Contains(t, publisher.published, "workflow.exchange/workflow.jobs.resume.CODE_REVIEW")
	assert.True(t, inboxRepo.completedMessages["stage.completed:ast.task.completed:repo_test_123:task_abc"])
}

func TestWorkflowJobQueueServiceEnqueue(t *testing.T) {
	jobRepo := &mockJobRepo{
		jobs:    make(map[uuid.UUID]*workflow.WorkflowJobModel),
		updates: make(map[uuid.UUID]map[string]any),
	}
	queueSvc := workflow.NewWorkflowJobQueueService(nil, nil, jobRepo)

	job := &workflow.WorkflowJobModel{
		WorkflowType: domain.WorkflowTypeCodeReview,
		HandlerType:  domain.HandlerTypePipelineSync,
	}

	jobID, err := queueSvc.Enqueue(context.Background(), job)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, jobID)
	assert.Equal(t, domain.JobStatusPending, job.Status)
	assert.Equal(t, 5, job.MaxRetries)

	// Cancel
	err = queueSvc.Cancel(context.Background(), jobID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusCancelled, jobRepo.updates[jobID]["status"])
}
