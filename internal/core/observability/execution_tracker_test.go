package observability_test

import (
	"errors"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionTrackerLifecycle(t *testing.T) {
	tracker := observability.NewExecutionTracker(100)

	execID := tracker.StartExecution(
		"drixy-reviewer",
		"corr_test_123",
		observability.ExecutionCycleMetadata{
			TenantID: "tenant_org_456",
			UserID:   "user_dev_789",
		},
		map[string]any{"pr": 42},
	)

	require.NotEmpty(t, execID)

	// Add steps
	tracker.AddStep(execID, observability.StepTypeLLMCall, "llm-client", map[string]any{
		"model":  "claude-3-5-sonnet",
		"tokens": 450,
	}, 120)

	tracker.AddStep(execID, observability.StepTypeValidation, "ast-syntax-validator", map[string]any{
		"valid": true,
	}, 15)

	// Complete
	tracker.CompleteExecution(execID, map[string]any{"suggestions": 3})

	cycle, found := tracker.GetExecution(execID)
	assert.True(t, found)
	require.NotNil(t, cycle)

	assert.Equal(t, "completed", cycle.Status)
	assert.Equal(t, "drixy-reviewer", cycle.AgentName)
	assert.Equal(t, "tenant_org_456", cycle.Metadata.TenantID)
	assert.True(t, len(cycle.Steps) >= 4) // start + 2 intermediate + complete
	assert.True(t, cycle.Duration >= 0)
}

func TestExecutionTrackerFailure(t *testing.T) {
	tracker := observability.NewExecutionTracker(100)

	execID := tracker.StartExecution("drixy-rules-engine", "corr_fail_1", observability.ExecutionCycleMetadata{}, nil)
	tracker.FailExecution(execID, errors.New("timeout querying pgvector embeddings"))

	cycle, found := tracker.GetExecution(execID)
	assert.True(t, found)
	assert.Equal(t, "failed", cycle.Status)
	assert.Contains(t, cycle.Error, "pgvector embeddings")
}

func TestExecutionTrackerMemoryBounds(t *testing.T) {
	// Max cycles = 3
	tracker := observability.NewExecutionTracker(3)

	id1 := tracker.StartExecution("agent", "c1", observability.ExecutionCycleMetadata{}, nil)
	time.Sleep(2 * time.Millisecond)
	_ = tracker.StartExecution("agent", "c2", observability.ExecutionCycleMetadata{}, nil)
	time.Sleep(2 * time.Millisecond)
	_ = tracker.StartExecution("agent", "c3", observability.ExecutionCycleMetadata{}, nil)
	time.Sleep(2 * time.Millisecond)
	id4 := tracker.StartExecution("agent", "c4", observability.ExecutionCycleMetadata{}, nil)

	// id1 should have been evicted
	_, found1 := tracker.GetExecution(id1)
	assert.False(t, found1, "oldest cycle should be evicted")

	_, found4 := tracker.GetExecution(id4)
	assert.True(t, found4)
}
