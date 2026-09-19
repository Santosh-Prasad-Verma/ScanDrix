package workflow_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnqueueCodeReviewJobUseCaseValidationAndExecution(t *testing.T) {
	queueService := workflow.NewWorkflowJobQueueService(nil, nil, nil)
	useCase := workflow.NewEnqueueCodeReviewJobUseCase(queueService)

	// Missing fields validation
	_, err := useCase.Execute(context.Background(), workflow.EnqueueCodeReviewInput{
		WorkspaceID: "",
	})
	assert.Error(t, err)

	// Valid input
	jobID, err := useCase.Execute(context.Background(), workflow.EnqueueCodeReviewInput{
		TenantID:     "tenant_abc",
		WorkspaceID:  "ws_123",
		RepositoryID: "repo_xyz",
		PRNumber:     101,
		CommitSHA:    "abc123def456",
		Title:        "Feature: Add auth SSO",
		Author:       "developer",
		Priority:     8,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, jobID)
}
