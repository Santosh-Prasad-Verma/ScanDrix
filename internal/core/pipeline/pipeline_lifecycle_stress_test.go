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

// ============================================================================
// Pipeline Lifecycle, DAG Execution & Check Run Matrix Test Suite
// ============================================================================

type lifecycleTestStage struct {
	name      string
	silent    bool
	fail      bool
	pause     bool
	pauseEvt  string
	pauseKey  string
	jumpTo    string
	skip      []string
	execFn    func(ctx context.Context, pCtx *pipeline.BasePipelineContext) error
	execCount int32
}

func (s *lifecycleTestStage) StageName() string              { return s.name }
func (s *lifecycleTestStage) IsSilent() bool                 { return s.silent }
func (s *lifecycleTestStage) Options() pipeline.StageOptions { return pipeline.StageOptions{Label: s.name} }
func (s *lifecycleTestStage) Execute(ctx context.Context, pCtx *pipeline.BasePipelineContext) error {
	atomic.AddInt32(&s.execCount, 1)

	if len(s.skip) > 0 {
		pCtx.StatusInfo.SkipStages = append(pCtx.StatusInfo.SkipStages, s.skip...)
	}
	if s.jumpTo != "" {
		pCtx.StatusInfo.JumpToStage = s.jumpTo
		pCtx.StatusInfo.Status = domain.AutomationStatusSkipped
	}
	if s.pause {
		return &pipeline.WorkflowPausedError{
			EventType: s.pauseEvt,
			EventKey:  s.pauseKey,
			StageName: s.name,
			TimeoutMs: 60000,
		}
	}
	if s.fail {
		return errors.New("deliberate stage failure: " + s.name)
	}
	if s.execFn != nil {
		return s.execFn(ctx, pCtx)
	}
	return nil
}

type fullLifecycleObserver struct {
	mu            sync.Mutex
	startedCount  int
	endedCount    int
	stageStarts   []string
	stageCompletes []string
	stageErrors   []string
}

func (o *fullLifecycleObserver) OnPipelineStart(ctx context.Context, pCtx *pipeline.BasePipelineContext) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.startedCount++
}

func (o *fullLifecycleObserver) OnStageStart(ctx context.Context, stageName string, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stageStarts = append(o.stageStarts, stageName)
}

func (o *fullLifecycleObserver) OnStageCompleted(ctx context.Context, stageName string, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stageCompletes = append(o.stageCompletes, stageName)
}

func (o *fullLifecycleObserver) OnStageError(ctx context.Context, stageName string, err error, pCtx *pipeline.BasePipelineContext, opts pipeline.StageOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stageErrors = append(o.stageErrors, stageName+":"+err.Error())
}

func (o *fullLifecycleObserver) OnPipelineEnd(ctx context.Context, pCtx *pipeline.BasePipelineContext) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.endedCount++
}

// TestPipelineLifecycle_SequentialExecution verifies end-to-end execution of a 6-stage review pipeline.
func TestPipelineLifecycle_SequentialExecution(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	observer := &fullLifecycleObserver{}

	stages := []pipeline.PipelineStage{
		&lifecycleTestStage{name: "InitStage"},
		&lifecycleTestStage{name: "FetchPRFilesStage"},
		&lifecycleTestStage{name: "FilterFilesStage"},
		&lifecycleTestStage{name: "AnalyzeChangesStage"},
		&lifecycleTestStage{name: "PRLevelReviewStage"},
		&lifecycleTestStage{name: "FileAnalysisStage"},
	}

	pCtx := &pipeline.BasePipelineContext{}
	outCtx, err := executor.Execute(context.Background(), pCtx, stages, "StandardReviewPipeline", observer)

	require.NoError(t, err)
	assert.Equal(t, domain.AutomationStatusSuccess, outCtx.StatusInfo.Status)
	assert.Empty(t, outCtx.Errors)

	// Verify observer invocations
	assert.Equal(t, 1, observer.startedCount)
	assert.Equal(t, 1, observer.endedCount)
	assert.Len(t, observer.stageStarts, 6)
	assert.Len(t, observer.stageCompletes, 6)
	assert.Empty(t, observer.stageErrors)

	expectedOrder := []string{"InitStage", "FetchPRFilesStage", "FilterFilesStage", "AnalyzeChangesStage", "PRLevelReviewStage", "FileAnalysisStage"}
	assert.Equal(t, expectedOrder, observer.stageStarts)
	assert.Equal(t, expectedOrder, observer.stageCompletes)
}

// TestPipelineLifecycle_JumpToStage verifies unconditional control flow jumping over intermediate stages.
func TestPipelineLifecycle_JumpToStage(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	observer := &fullLifecycleObserver{}

	stages := []pipeline.PipelineStage{
		&lifecycleTestStage{name: "StageA", jumpTo: "StageD"},
		&lifecycleTestStage{name: "StageB"}, // should be skipped
		&lifecycleTestStage{name: "StageC"}, // should be skipped
		&lifecycleTestStage{name: "StageD"},
		&lifecycleTestStage{name: "StageE"},
	}

	pCtx := &pipeline.BasePipelineContext{}
	outCtx, err := executor.Execute(context.Background(), pCtx, stages, "JumpPipeline", observer)

	require.NoError(t, err)
	assert.Equal(t, domain.AutomationStatusSuccess, outCtx.StatusInfo.Status)

	// StageB and StageC should not have executed
	assert.Contains(t, observer.stageCompletes, "StageA")
	assert.NotContains(t, observer.stageCompletes, "StageB")
	assert.NotContains(t, observer.stageCompletes, "StageC")
	assert.Contains(t, observer.stageCompletes, "StageD")
	assert.Contains(t, observer.stageCompletes, "StageE")
}

// TestPipelineLifecycle_SkipStagesList verifies explicit stage skipping via statusInfo.skipStages.
func TestPipelineLifecycle_SkipStagesList(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	observer := &fullLifecycleObserver{}

	stages := []pipeline.PipelineStage{
		&lifecycleTestStage{name: "PlannerStage", skip: []string{"HeavyASTStage", "OptionalLintStage"}},
		&lifecycleTestStage{name: "HeavyASTStage"},     // skipped
		&lifecycleTestStage{name: "CoreReviewStage"},   // executed
		&lifecycleTestStage{name: "OptionalLintStage"}, // skipped
		&lifecycleTestStage{name: "SummaryStage"},      // executed
	}

	pCtx := &pipeline.BasePipelineContext{}
	outCtx, err := executor.Execute(context.Background(), pCtx, stages, "SkipListPipeline", observer)

	require.NoError(t, err)
	assert.Equal(t, domain.AutomationStatusSuccess, outCtx.StatusInfo.Status)

	assert.Contains(t, observer.stageCompletes, "PlannerStage")
	assert.NotContains(t, observer.stageCompletes, "HeavyASTStage")
	assert.Contains(t, observer.stageCompletes, "CoreReviewStage")
	assert.NotContains(t, observer.stageCompletes, "OptionalLintStage")
	assert.Contains(t, observer.stageCompletes, "SummaryStage")
}

// TestPipelineLifecycle_WorkflowPausedErrorHandling verifies pausing on asynchronous heavy stages.
func TestPipelineLifecycle_WorkflowPausedErrorHandling(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	observer := &fullLifecycleObserver{}

	stages := []pipeline.PipelineStage{
		&lifecycleTestStage{name: "InitStage"},
		&lifecycleTestStage{
			name:     "AsyncASTStage",
			pause:    true,
			pauseEvt: "ast.task.completed",
			pauseKey: "repo:123:sha:abc",
		},
		&lifecycleTestStage{name: "SubsequentReviewStage"}, // should not execute
	}

	pCtx := &pipeline.BasePipelineContext{}
	outCtx, err := executor.Execute(context.Background(), pCtx, stages, "PausePipeline", observer)

	// When a stage returns WorkflowPausedError, executor captures it in outCtx.Errors and marks status failure
	require.NoError(t, err)
	assert.Equal(t, domain.AutomationStatusFailure, outCtx.StatusInfo.Status)
	require.NotEmpty(t, outCtx.Errors)
	assert.Contains(t, outCtx.Errors[0].Error, "ast.task.completed")
	assert.Contains(t, outCtx.Errors[0].Error, "repo:123:sha:abc")
	assert.Equal(t, "AsyncASTStage", outCtx.Errors[0].Stage)

	// Subsequent stage should not have been started
	assert.NotContains(t, observer.stageStarts, "SubsequentReviewStage")
}

// TestPipelineLifecycle_StageFailureAndErrorAccumulator verifies error tracking on failure.
func TestPipelineLifecycle_StageFailureAndErrorAccumulator(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	observer := &fullLifecycleObserver{}

	stages := []pipeline.PipelineStage{
		&lifecycleTestStage{name: "StagePass1"},
		&lifecycleTestStage{name: "StageFail", fail: true},
		&lifecycleTestStage{name: "StagePass2"}, // skipped because status became FAILURE
	}

	pCtx := &pipeline.BasePipelineContext{}
	outCtx, err := executor.Execute(context.Background(), pCtx, stages, "FailurePipeline", observer)

	require.NoError(t, err, "executor records errors in context rather than returning them directly")
	assert.Equal(t, domain.AutomationStatusFailure, outCtx.StatusInfo.Status)
	assert.Len(t, outCtx.Errors, 1)
	assert.Equal(t, "StageFail", outCtx.Errors[0].Stage)
	assert.Equal(t, domain.PipelineSeverityHigh, outCtx.Errors[0].Severity)
	assert.Contains(t, outCtx.Errors[0].Error, "deliberate stage failure: StageFail")

	// Observer should have recorded the error
	assert.Len(t, observer.stageErrors, 1)
	assert.Contains(t, observer.stageErrors[0], "StageFail")
	assert.Equal(t, 1, observer.endedCount)
}

// TestPipelineLifecycle_ContextCancellationMidPipeline verifies clean abort on timeout or cancellation.
func TestPipelineLifecycle_ContextCancellationMidPipeline(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	ctx, cancel := context.WithCancel(context.Background())

	stages := []pipeline.PipelineStage{
		&lifecycleTestStage{name: "Stage1", execFn: func(c context.Context, p *pipeline.BasePipelineContext) error {
			cancel() // cancel context immediately after stage 1
			return nil
		}},
		&lifecycleTestStage{name: "Stage2", execFn: func(c context.Context, p *pipeline.BasePipelineContext) error {
			select {
			case <-c.Done():
				return c.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		}},
	}

	pCtx := &pipeline.BasePipelineContext{}
	outCtx, err := executor.Execute(ctx, pCtx, stages, "CanceledPipeline")

	require.NoError(t, err)
	assert.Equal(t, domain.AutomationStatusFailure, outCtx.StatusInfo.Status)
	assert.Len(t, outCtx.Errors, 1)
	assert.Equal(t, "Stage2", outCtx.Errors[0].Stage)
}

// TestPipelineLifecycle_ConcurrentPipelineExecutions verifies 50 parallel pipelines execute safely without races.
func TestPipelineLifecycle_ConcurrentPipelineExecutions(t *testing.T) {
	executor := pipeline.NewPipelineExecutor()
	const numPipelines = 50

	var wg sync.WaitGroup
	wg.Add(numPipelines)

	var successCount atomic.Int64

	for i := 0; i < numPipelines; i++ {
		go func(pIndex int) {
			defer wg.Done()
			pCtx := &pipeline.BasePipelineContext{
				CorrelationID: fmt.Sprintf("corr-%d", pIndex),
			}

			stages := []pipeline.PipelineStage{
				&lifecycleTestStage{name: "StepA"},
				&lifecycleTestStage{name: "StepB"},
				&lifecycleTestStage{name: "StepC"},
			}

			outCtx, err := executor.Execute(
				context.Background(),
				pCtx,
				stages,
				fmt.Sprintf("ParallelPipeline-%d", pIndex),
			)
			if err == nil && outCtx.StatusInfo.Status == domain.AutomationStatusSuccess {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(numPipelines), successCount.Load())
}

// TestPipelineLifecycle_ChecksServiceTitleAndSummaryMatrix verifies check run titles and summaries.
func TestPipelineLifecycle_ChecksServiceTitleAndSummaryMatrix(t *testing.T) {
	// Verify CheckStageMap mappings
	startInfo, ok := pipeline.CheckStageMap["_pipelineStart"]
	require.True(t, ok)
	assert.Equal(t, "Code Review Starting", startInfo.Title)
	assert.Contains(t, startInfo.Summary, "Drixy is analyzing")

	prInfo, ok := pipeline.CheckStageMap["PRLevelReviewStage"]
	require.True(t, ok)
	assert.Equal(t, "Code Review In Progress", prInfo.Title)

	fileInfo, ok := pipeline.CheckStageMap["FileAnalysisStage"]
	require.True(t, ok)
	assert.Equal(t, "Code Review In Progress", fileInfo.Title)

	endSuccessInfo, ok := pipeline.CheckStageMap["_pipelineEndSuccess"]
	require.True(t, ok)
	assert.Equal(t, "Code Review Complete", endSuccessInfo.Title)

	// Check status constants
	assert.Equal(t, pipeline.CheckStatus("queued"), pipeline.CheckStatusQueued)
	assert.Equal(t, pipeline.CheckStatus("in_progress"), pipeline.CheckStatusInProgress)
	assert.Equal(t, pipeline.CheckStatus("completed"), pipeline.CheckStatusCompleted)

	// Check conclusion constants
	assert.Equal(t, pipeline.CheckConclusion("success"), pipeline.CheckConclusionSuccess)
	assert.Equal(t, pipeline.CheckConclusion("failure"), pipeline.CheckConclusionFailure)
	assert.Equal(t, pipeline.CheckConclusion("neutral"), pipeline.CheckConclusionNeutral)
	assert.Equal(t, pipeline.CheckConclusion("action_required"), pipeline.CheckConclusionActionRequired)
	assert.Equal(t, pipeline.CheckConclusion("skipped"), pipeline.CheckConclusionSkipped)
	assert.Equal(t, pipeline.CheckConclusion("timed_out"), pipeline.CheckConclusionTimedOut)
	assert.Equal(t, pipeline.CheckConclusion("cancelled"), pipeline.CheckConclusionCancelled)
}
