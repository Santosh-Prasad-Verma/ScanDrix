package workflow_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateSerializerFullAndCompressed(t *testing.T) {
	serializer := workflow.NewStateSerializer()

	// 1. Full small state
	state := map[string]any{
		"workflowJobId": "job_123",
		"currentStage":  "FileAnalysisStage",
		"prNumber":      42,
	}

	serialized, err := serializer.Serialize(state, workflow.SerializationOptions{
		Strategy: workflow.StrategyFull,
	})
	require.NoError(t, err)
	assert.Equal(t, "job_123", serialized["workflowJobId"])

	deserialized, err := serializer.Deserialize(serialized)
	require.NoError(t, err)
	assert.Equal(t, "job_123", deserialized["workflowJobId"])

	// 2. Compressed large state (> 50KB)
	largeDiff := strings.Repeat("diff --git a/main.go b/main.go\n+ func Review() {}\n", 2000)
	largeState := map[string]any{
		"workflowJobId": "job_heavy_456",
		"rawDiff":       largeDiff,
	}

	compressed, err := serializer.Serialize(largeState, workflow.SerializationOptions{
		Strategy: workflow.StrategyCompressed,
	})
	require.NoError(t, err)
	assert.True(t, compressed["compressed"].(bool))
	assert.NotEmpty(t, compressed["data"])

	unpacked, err := serializer.Deserialize(compressed)
	require.NoError(t, err)
	assert.Equal(t, "job_heavy_456", unpacked["workflowJobId"])
	assert.Equal(t, largeDiff, unpacked["rawDiff"])
}

func TestStateSerializerMinimalAndDelta(t *testing.T) {
	serializer := workflow.NewStateSerializer()

	state1 := map[string]any{
		"workflowJobId": "job_delta_1",
		"currentStage":  "StageA",
		"heavyASTData":  "large_ast_tree",
		"prNumber":      10,
	}

	state2 := map[string]any{
		"workflowJobId": "job_delta_1",
		"currentStage":  "StageB", // changed
		"heavyASTData":  "large_ast_tree", // unchanged
		"newFinding":    "N+1 query", // added
		"prNumber":      10,
	}

	// Test Delta
	delta, err := serializer.Serialize(state2, workflow.SerializationOptions{
		Strategy:      workflow.StrategyDelta,
		PreviousState: state1,
	})
	require.NoError(t, err)
	assert.Equal(t, "StageB", delta["currentStage"])
	assert.Equal(t, "N+1 query", delta["newFinding"])
	_, hasUnchanged := delta["heavyASTData"]
	assert.False(t, hasUnchanged, "unchanged large AST data should be omitted in delta")

	// Test Minimal
	minimal, err := serializer.Serialize(state1, workflow.SerializationOptions{
		Strategy: workflow.StrategyMinimal,
	})
	require.NoError(t, err)
	assert.Equal(t, "job_delta_1", minimal["workflowJobId"])
	_, hasHeavy := minimal["heavyASTData"]
	assert.False(t, hasHeavy, "minimal strategy should omit heavyASTData")
}
