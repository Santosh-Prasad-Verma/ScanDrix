package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrossProcessEventsBridgeSubscriptionAndForward(t *testing.T) {
	bridge := workflow.NewCrossProcessEventsBridge(nil, "")

	receivedCount := 0
	eventName := "pull-request.closed"

	bridge.Subscribe(eventName, func(ctx context.Context, name string, payload map[string]any) error {
		receivedCount++
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	payload := map[string]any{
		"repoId": "repo_123",
		"pr":     42,
	}

	err := bridge.Forward(ctx, eventName, payload)
	require.NoError(t, err)

	metrics := bridge.GetMetrics()
	assert.Equal(t, uint64(1), metrics["forwarded"])

	// Test BridgedFlag loop guard
	loopPayload := map[string]any{
		"repoId":                     "repo_123",
		workflow.BridgedFlag: true,
	}
	err = bridge.Forward(ctx, eventName, loopPayload)
	require.NoError(t, err)
	// forwarded count should stay 1 because bridged flag stopped re-forwarding
	metrics = bridge.GetMetrics()
	assert.Equal(t, uint64(1), metrics["forwarded"])
}
