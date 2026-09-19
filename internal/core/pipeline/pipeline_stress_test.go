package pipeline_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type concurrentStage struct {
	name      string
	silent    bool
	fail      bool
	delay     time.Duration
	execCount atomic.Int32
}

func (s *concurrentStage) StageName() string              { return s.name }
func (s *concurrentStage) IsSilent() bool                 { return s.silent }
func (s *concurrentStage) Options() pipeline.StageOptions { return pipeline.StageOptions{Label: s.name} }
func (s *concurrentStage) Execute(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
	s.execCount.Add(1)
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.delay):
		}
	}
	if s.fail {
		return errors.New("simulated stage failure")
	}
	return nil
}

type recordingObserver struct {
	mu     sync.Mutex
	events []string
}

func (o *recordingObserver) OnPipelineStart(ctx context.Context, pCtx *pipeline.BasePipelineContext) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, "start:"+pCtx.PipelineMetadata.PipelineID)
}

func (o *recordingObserver) OnStageStart(ctx context.Context, stageName string, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, "stage_start:"+stageName)
}

func (o *recordingObserver) OnStageCompleted(ctx context.Context, stageName string, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, "stage_done:"+stageName)
}

func (o *recordingObserver) OnStageError(ctx context.Context, stageName string, err error, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, "stage_error:"+stageName+":"+err.Error())
}

func (o *recordingObserver) OnPipelineEnd(ctx context.Context, pCtx *pipeline.BasePipelineContext) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, "end:"+string(pCtx.StatusInfo.Status))
}

// TestPipelineStress_HighConcurrencyExecution runs 40 independent pipelines
// concurrently across goroutines, ensuring zero race conditions and correct isolated contexts.
func TestPipelineStress_HighConcurrencyExecution(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	const numPipelines = 40
	var completedPipelines atomic.Int32

	var wg sync.WaitGroup
	wg.Add(numPipelines)

	for p := 0; p < numPipelines; p++ {
		go func(pipeIdx int) {
			defer wg.Done()
			obs := &recordingObserver{}

			stages := []pipeline.PipelineStage{
				&concurrentStage{name: "stage_init", delay: time.Millisecond},
				&concurrentStage{name: "stage_ast", delay: 2 * time.Millisecond},
				&concurrentStage{name: "stage_review", delay: time.Millisecond},
				&concurrentStage{name: "stage_persist", delay: time.Millisecond},
			}

			pCtx := &pipeline.BasePipelineContext{
				CorrelationID: fmt.Sprintf("corr-%d", pipeIdx),
			}

			resCtx, err := executor.Execute(context.Background(), pCtx, stages, fmt.Sprintf("Pipeline-%d", pipeIdx), obs)
			assert.NoError(t, err)
			assert.Equal(t, domain.AutomationStatusSuccess, resCtx.StatusInfo.Status)
			assert.NotEmpty(t, resCtx.PipelineMetadata.PipelineID)
			assert.Equal(t, fmt.Sprintf("Pipeline-%d", pipeIdx), resCtx.PipelineMetadata.PipelineName)
			completedPipelines.Add(1)
		}(p)
	}

	wg.Wait()
	assert.Equal(t, int32(numPipelines), completedPipelines.Load())
}

// TestPipelineStress_JumpToStageWorkflow validates dynamic jumpToStage control flow:
// skipping intervening stages and resuming at a designated target stage.
func TestPipelineStress_JumpToStageWorkflow(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()

	s1 := &concurrentStage{name: "stage_1"}
	s2 := &concurrentStage{name: "stage_2"} // should be skipped
	s3 := &concurrentStage{name: "stage_3"} // target of jump
	s4 := &concurrentStage{name: "stage_4"}

	// Custom executeFn on stage_1 triggers jumpToStage
	stage1 := &testStage{
		name: "stage_1",
		executeFn: func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
			pCtx.StatusInfo.Status = domain.AutomationStatusSkipped
			pCtx.StatusInfo.JumpToStage = "stage_3"
			return nil
		},
	}

	stages := []pipeline.PipelineStage{stage1, s2, s3, s4}
	pCtx := &pipeline.BasePipelineContext{}

	resCtx, err := executor.Execute(context.Background(), pCtx, stages, "JumpPipeline")
	require.NoError(t, err)

	// Stage 2 should not have run
	assert.Equal(t, int32(0), s2.execCount.Load())

	// Stage 3 and Stage 4 should have executed
	assert.Equal(t, int32(1), s3.execCount.Load())
	assert.Equal(t, int32(1), s4.execCount.Load())

	assert.Equal(t, domain.AutomationStatusSuccess, resCtx.StatusInfo.Status)
	assert.Equal(t, "", resCtx.StatusInfo.JumpToStage)
	_ = s1
}

// TestPipelineStress_FailureHaltsSubsequentStages verifies that when a stage fails,
// subsequent stages are not executed, error is recorded, and status is set to FAILURE.
func TestPipelineStress_FailureHaltsSubsequentStages(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	obs := &recordingObserver{}

	s1 := &concurrentStage{name: "fetch_diff"}
	s2 := &concurrentStage{name: "failing_stage", fail: true}
	s3 := &concurrentStage{name: "post_comments"} // should never execute

	stages := []pipeline.PipelineStage{s1, s2, s3}
	pCtx := &pipeline.BasePipelineContext{}

	resCtx, err := executor.Execute(context.Background(), pCtx, stages, "FailPipeline", obs)
	require.NoError(t, err) // Execute returns pCtx with status and errors, does not return raw stage err

	assert.Equal(t, domain.AutomationStatusFailure, resCtx.StatusInfo.Status)
	assert.Len(t, resCtx.Errors, 1)
	assert.Equal(t, "failing_stage", resCtx.Errors[0].Stage)
	assert.Equal(t, "simulated stage failure", resCtx.Errors[0].Error)
	assert.Equal(t, domain.PipelineSeverityHigh, resCtx.Errors[0].Severity)

	// Stage 3 never executed
	assert.Equal(t, int32(0), s3.execCount.Load())

	// Observer notified of stage error and failure end
	obs.mu.Lock()
	defer obs.mu.Unlock()
	assert.Contains(t, obs.events, "stage_error:failing_stage:simulated stage failure")
	assert.Contains(t, obs.events, "end:FAILURE")
}

// TestPipelineStress_SilentStageSuppressesObserver confirms that silent stages
// do not dispatch OnStageStart, OnStageCompleted, or OnStageError events to observers.
func TestPipelineStress_SilentStageSuppressesObserver(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	obs := &recordingObserver{}

	sSilent := &concurrentStage{name: "telemetry_heartbeat", silent: true}
	sPublic := &concurrentStage{name: "public_step", silent: false}

	stages := []pipeline.PipelineStage{sSilent, sPublic}
	pCtx := &pipeline.BasePipelineContext{}

	_, err := executor.Execute(context.Background(), pCtx, stages, "SilentPipeline", obs)
	require.NoError(t, err)

	obs.mu.Lock()
	defer obs.mu.Unlock()

	// Public step events are recorded
	assert.Contains(t, obs.events, "stage_start:public_step")
	assert.Contains(t, obs.events, "stage_done:public_step")

	// Silent step events are absent
	for _, ev := range obs.events {
		assert.NotContains(t, ev, "telemetry_heartbeat")
	}
}

// TestPipelineStress_ChecksStageMapCompleteness validates the predefined check stages
// and message summaries.
func TestPipelineStress_ChecksStageMapCompleteness(t *testing.T) {
	expectedStages := []string{
		"_pipelineStart",
		"PRLevelReviewStage",
		"FileAnalysisStage",
		"_pipelineEndSuccess",
	}

	for _, stage := range expectedStages {
		info, exists := pipeline.CheckStageMap[stage]
		assert.True(t, exists, "Expected stage info for: %s", stage)
		assert.NotEmpty(t, info.Title)
		assert.NotEmpty(t, info.Summary)
	}
}
