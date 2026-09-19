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

type testStage struct {
	name      string
	silent    bool
	executeFn func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error
}

func (s *testStage) StageName() string              { return s.name }
func (s *testStage) IsSilent() bool                 { return s.silent }
func (s *testStage) Options() pipeline.StageOptions { return pipeline.StageOptions{Label: s.name} }
func (s *testStage) Execute(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
	if s.executeFn != nil {
		return s.executeFn(ctx, pCtx)
	}
	return nil
}

type testObserver struct {
	events []string
}

func (o *testObserver) OnPipelineStart(ctx context.Context, pCtx *pipeline.BasePipelineContext) {
	o.events = append(o.events, "start")
}
func (o *testObserver) OnStageStart(ctx context.Context, stageName string, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.events = append(o.events, "stage_start:"+stageName)
}
func (o *testObserver) OnStageCompleted(ctx context.Context, stageName string, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.events = append(o.events, "stage_done:"+stageName)
}
func (o *testObserver) OnStageError(ctx context.Context, stageName string, err error, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.events = append(o.events, "stage_err:"+stageName)
}
func (o *testObserver) OnPipelineEnd(ctx context.Context, pCtx *pipeline.BasePipelineContext) {
	o.events = append(o.events, "end")
}

func TestPipelineExecutorLifecycle(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	obs := &testObserver{}

	stage1Executed := false
	stage2Executed := false

	stages := []pipeline.PipelineStage{
		&testStage{
			name: "fetch_diff",
			executeFn: func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
				stage1Executed = true
				return nil
			},
		},
		&testStage{
			name: "analyze_ast",
			executeFn: func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
				stage2Executed = true
				return nil
			},
		},
	}

	pCtx := &pipeline.BasePipelineContext{}
	resCtx, err := executor.Execute(context.Background(), pCtx, stages, "CodeReviewPipeline", obs)

	require.NoError(t, err)
	assert.True(t, stage1Executed)
	assert.True(t, stage2Executed)
	assert.Equal(t, domain.AutomationStatusSuccess, resCtx.StatusInfo.Status)
	assert.Contains(t, obs.events, "start")
	assert.Contains(t, obs.events, "stage_start:fetch_diff")
	assert.Contains(t, obs.events, "stage_done:fetch_diff")
	assert.Contains(t, obs.events, "end")
}

func TestPipelineExecutorSkipStages(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()

	stage2Executed := false

	stages := []pipeline.PipelineStage{
		&testStage{name: "stage1"},
		&testStage{
			name: "stage2",
			executeFn: func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
				stage2Executed = true
				return nil
			},
		},
	}

	pCtx := &pipeline.BasePipelineContext{
		StatusInfo: pipeline.StatusInfo{
			SkipStages: []string{"stage2"},
		},
	}

	resCtx, err := executor.Execute(context.Background(), pCtx, stages, "SkipPipeline")
	require.NoError(t, err)
	assert.False(t, stage2Executed, "stage2 should have been skipped")
	assert.Equal(t, domain.AutomationStatusSuccess, resCtx.StatusInfo.Status)
}

func TestPipelineExecutorErrorHandling(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()

	stages := []pipeline.PipelineStage{
		&testStage{
			name: "failing_stage",
			executeFn: func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
				return errors.New("syntax parsing failure in ast")
			},
		},
		&testStage{name: "unreached_stage"},
	}

	pCtx := &pipeline.BasePipelineContext{}
	resCtx, err := executor.Execute(context.Background(), pCtx, stages, "FailPipeline")

	require.NoError(t, err)
	assert.Equal(t, domain.AutomationStatusFailure, resCtx.StatusInfo.Status)
	assert.Len(t, resCtx.Errors, 1)
	assert.Equal(t, "failing_stage", resCtx.Errors[0].Stage)
	assert.Contains(t, resCtx.Errors[0].Error, "syntax parsing failure")
}
