package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWaitingRepo struct {
	waitingJobs []*workflow.WorkflowJobModel
}

func (m *mockWaitingRepo) FindWaitingJobsByEvent(ctx context.Context, eventType, eventKey string) ([]*workflow.WorkflowJobModel, error) {
	return m.waitingJobs, nil
}

func (m *mockWaitingRepo) FindOne(ctx context.Context, id uuid.UUID) (*workflow.WorkflowJobModel, error) {
	for _, j := range m.waitingJobs {
		if j.UUID == id {
			return j, nil
		}
	}
	return nil, nil
}

func (m *mockWaitingRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	for _, j := range m.waitingJobs {
		if j.UUID == id {
			if s, ok := updates["status"].(domain.JobStatus); ok {
				j.Status = s
			}
		}
	}
	return nil
}

func TestASTEventHandlerResumption(t *testing.T) {
	jobID := uuid.New()
	job := &workflow.WorkflowJobModel{
		UUID:         jobID,
		Status:       domain.JobStatusWaitingForEvent,
		WorkflowType: domain.WorkflowTypeCodeReview,
		Payload:      map[string]any{"pr": 42},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	waitingRepo := &mockWaitingRepo{
		waitingJobs: []*workflow.WorkflowJobModel{job},
	}
	inboxRepo := newMockInboxRepo()

	handler := workflow.NewASTEventHandler(waitingRepo, inboxRepo, nil, "worker-node-1")

	msg := workflow.ASTCompletedMessage{
		TaskID: "ast_task_999",
		Result: map[string]any{
			"symbolsCount": 450,
			"callGraph":    true,
		},
	}

	err := handler.HandleASTCompleted(context.Background(), msg, "msg_ast_1")
	require.NoError(t, err)

	assert.Equal(t, domain.JobStatusProcessing, job.Status)
	assert.NotNil(t, job.Payload["astResult"])
}
