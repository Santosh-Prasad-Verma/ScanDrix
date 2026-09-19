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

type mockStatusJobRepo struct {
	jobs map[uuid.UUID]*workflow.WorkflowJobModel
}

func (m *mockStatusJobRepo) FindOne(ctx context.Context, id uuid.UUID) (*workflow.WorkflowJobModel, error) {
	return m.jobs[id], nil
}

func (m *mockStatusJobRepo) FindByCorrelationID(ctx context.Context, correlationID string) (*workflow.WorkflowJobModel, error) {
	for _, j := range m.jobs {
		if j.CorrelationID == correlationID {
			return j, nil
		}
	}
	return nil, nil
}

func (m *mockStatusJobRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	j := m.jobs[id]
	if j != nil {
		if s, ok := updates["status"].(domain.JobStatus); ok {
			j.Status = s
		}
	}
	return nil
}

func TestJobStatusServiceProgressCalculation(t *testing.T) {
	jobID := uuid.New()
	stage := "PRLevelReviewStage"
	now := time.Now().UTC()

	repo := &mockStatusJobRepo{
		jobs: map[uuid.UUID]*workflow.WorkflowJobModel{
			jobID: {
				UUID:         jobID,
				Status:       domain.JobStatusProcessing,
				WorkflowType: domain.WorkflowTypeCodeReview,
				CurrentStage: &stage,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}

	service := workflow.NewJobStatusService(nil, repo)

	detail, err := service.GetJobDetail(context.Background(), jobID)
	require.NoError(t, err)
	require.NotNil(t, detail)

	assert.Equal(t, 70, detail.ProgressPercent)
	assert.Equal(t, domain.JobStatusProcessing, detail.Job.Status)

	// Completed should be 100%
	detail.Job.Status = domain.JobStatusCompleted
	detail2, _ := service.GetJobDetail(context.Background(), jobID)
	assert.Equal(t, 100, detail2.ProgressPercent)
}

func TestJobStatusServiceMetricsEmpty(t *testing.T) {
	service := workflow.NewJobStatusService(nil, nil)
	metrics, err := service.GetMetrics(context.Background())
	require.NoError(t, err)
	require.NotNil(t, metrics)
	assert.Equal(t, 100.0, metrics.SuccessRate)
}
