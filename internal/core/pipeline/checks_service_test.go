package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChecksAdapter struct {
	runs []*pipeline.CheckRunRecord
}

func (m *mockChecksAdapter) CreateOrUpdateCheckRun(ctx context.Context, check *pipeline.CheckRunRecord) (*pipeline.CheckRunRecord, error) {
	if check.ID == "" {
		check.ID = "check_run_123"
	}
	m.runs = append(m.runs, check)
	return check, nil
}

func TestPipelineChecksServiceLifecycle(t *testing.T) {
	mockAdapter := &mockChecksAdapter{}
	svc := pipeline.NewPipelineChecksService(mockAdapter)
	ctx := context.Background()

	// 1. Pipeline Start
	startRun, err := svc.UpdatePipelineStart(ctx, "commit_sha_1", "https://scandrix.dev/reviews/1")
	require.NoError(t, err)
	assert.Equal(t, pipeline.DrixyCheckRunName, startRun.Name)
	assert.Equal(t, pipeline.CheckStatusInProgress, startRun.Status)
	assert.Equal(t, "Code Review Starting", startRun.Title)

	// 2. Stage Progress
	progressRun, err := svc.UpdateStageProgress(ctx, startRun.ID, "commit_sha_1", "PRLevelReviewStage")
	require.NoError(t, err)
	assert.Equal(t, pipeline.CheckStatusInProgress, progressRun.Status)
	assert.Equal(t, "Code Review In Progress", progressRun.Title)

	// 3. Pipeline End Success
	endRun, err := svc.UpdatePipelineEnd(ctx, startRun.ID, "commit_sha_1", domain.AutomationStatusSuccess, nil)
	require.NoError(t, err)
	assert.Equal(t, pipeline.CheckStatusCompleted, endRun.Status)
	assert.Equal(t, pipeline.CheckConclusionSuccess, endRun.Conclusion)
	assert.Equal(t, "Code Review Complete", endRun.Title)
}

func TestStageMessageHelper(t *testing.T) {
	var helper pipeline.StageMessageHelper

	msg := helper.Skipped("File analysis bypassed", "No supported language files detected")
	assert.Equal(t, "File analysis bypassed (Tech: No supported language files detected)", msg)

	msgErr := helper.Error("Failed to fetch repository ast", errors.New("timeout connecting to language server"))
	assert.Equal(t, "Failed to fetch repository ast (Error: timeout connecting to language server)", msgErr)
}

func TestWorkflowPausedError(t *testing.T) {
	err := &pipeline.WorkflowPausedError{
		EventType: "ast.task.completed",
		EventKey:  "repo:123:sha:abc",
		TimeoutMs: 60000,
		StageName: "ASTAnalysisStage",
	}

	assert.Contains(t, err.Error(), "ASTAnalysisStage")
	assert.Contains(t, err.Error(), "ast.task.completed")
	assert.Contains(t, err.Error(), "60000ms")
}
