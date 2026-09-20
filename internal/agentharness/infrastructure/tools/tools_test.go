// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyTool struct {
	name      string
	callCount int
	isError   bool
}

func (d *dummyTool) Name() string {
	return d.name
}

func (d *dummyTool) Description() string {
	return "dummy tool"
}

func (d *dummyTool) InputSchema() contracts.JSONSchema {
	return contracts.JSONSchema{Type: "object"}
}

func (d *dummyTool) Strict() bool {
	return false
}

func (d *dummyTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	d.callCount++
	return contracts.ToolResult{
		Output:  "dummy output",
		IsError: d.isError,
	}, nil
}

func TestInMemoryToolRegistry(t *testing.T) {
	t1 := &dummyTool{name: "readFile"}
	t2 := &dummyTool{name: "grep"}

	reg := tools.NewInMemoryToolRegistry(t1, t2)

	assert.Equal(t, 2, len(reg.List()))
	found, ok := reg.Get("readFile")
	assert.True(t, ok)
	assert.Equal(t, "readFile", found.Name())

	_, ok = reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestCachingTool(t *testing.T) {
	inner := &dummyTool{name: "readFile"}
	cache := tools.NewToolCallCache()
	caching := tools.NewCachingTool(inner, cache)

	ctx := contracts.ToolContext{
		RunID:   "run-123",
		Context: context.Background(),
	}

	// 1st call -> executed
	res1, err := caching.Execute(ctx, map[string]any{"path": "foo/bar.go"})
	require.NoError(t, err)
	assert.Equal(t, "dummy output", res1.Output)
	assert.Equal(t, 1, inner.callCount)
	assert.Equal(t, 1, cache.Stats().Misses)
	assert.Equal(t, 0, cache.Stats().Hits)

	// 2nd call identical arguments (keys reordered) -> served from cache!
	res2, err := caching.Execute(ctx, map[string]any{"path": "foo/bar.go"})
	require.NoError(t, err)
	assert.Equal(t, "dummy output", res2.Output)
	assert.Equal(t, 1, inner.callCount, "underlying tool must NOT be invoked again")
	assert.Equal(t, 1, cache.Stats().Hits)
	assert.True(t, res2.Meta["cached"].(bool))

	// Cross-run isolation: different runID -> executed fresh!
	ctx2 := contracts.ToolContext{RunID: "run-456", Context: context.Background()}
	_, err = caching.Execute(ctx2, map[string]any{"path": "foo/bar.go"})
	require.NoError(t, err)
	assert.Equal(t, 2, inner.callCount, "different runID must not leak across runs")

	// Error tool execution -> never cached
	initialSize := cache.Stats().Size
	errTool := &dummyTool{name: "failing", isError: true}
	cachingErr := tools.NewCachingTool(errTool, cache)
	_, _ = cachingErr.Execute(ctx, "arg")
	assert.Equal(t, initialSize, cache.Stats().Size, "failing results must not be remembered in cache")
}
