// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package compression_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/compression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenEstimator(t *testing.T) {
	text := "func CalculateTax(amount float64) float64"
	tokens := compression.EstimateTextTokens(text)
	assert.Greater(t, tokens, 5)

	valTokens := compression.EstimateValueTokens(map[string]any{
		"path": "internal/auth/password.go",
		"line": 42,
	})
	assert.Greater(t, valTokens, 5)

	overhead := compression.EstimateOverheadTokens("You are an expert reviewer.", []any{"readFile", "grep"})
	assert.Greater(t, overhead, 10)
}

func TestContextCompressor(t *testing.T) {
	longOutput := strings.Repeat("A", 4000)
	messages := []contracts.AgentMessage{
		{Role: contracts.RoleSystem, Content: "System prompt"},
		{Role: contracts.RoleUser, Content: "<Diffs> Important diff content </Diffs>"},
		{
			Role: contracts.RoleTool,
			ToolCalls: []contracts.ToolCallRecord{
				{Name: "readFile", Output: longOutput},
			},
		},
		{
			Role: contracts.RoleTool,
			ToolCalls: []contracts.ToolCallRecord{
				{Name: "checkTypes", Output: "recent output"},
			},
		},
	}

	compressed := compression.CompressMessages(messages, []contracts.ToolCallRecord{
		{Name: "readFile", Output: "long output summary"},
	})

	// Head preserved
	assert.Equal(t, contracts.RoleSystem, compressed[0].Role)
	assert.Equal(t, contracts.RoleUser, compressed[1].Role)

	// Summary injected
	assert.True(t, len(compressed) >= 4)

	// Clamp to strict budget
	clamped := compression.ClampMessagesToBudget(messages, 50)
	clampedTokens := compression.EstimateMessagesTokens(clamped)
	assert.LessOrEqual(t, clampedTokens, 150)
}

func TestContextWindowCompressor(t *testing.T) {
	compressor := compression.NewContextWindowCompressor(1000, compression.ContextWindowCompressorOptions{
		OverheadTokens: 100,
	})

	smallMessages := []contracts.AgentMessage{
		{Role: contracts.RoleUser, Content: "short prompt"},
	}
	// Small message history -> no compression needed
	res := compressor.MaybeCompress(smallMessages)
	assert.Nil(t, res)

	// Huge message history exceeding window -> compressed
	hugeMessages := []contracts.AgentMessage{
		{Role: contracts.RoleSystem, Content: "sys"},
		{Role: contracts.RoleUser, Content: "user"},
	}
	for i := 0; i < 20; i++ {
		hugeMessages = append(hugeMessages, contracts.AgentMessage{
			Role: contracts.RoleTool,
			ToolCalls: []contracts.ToolCallRecord{
				{Name: "readFile", Output: strings.Repeat("huge output text ", 50)},
			},
		})
	}

	res = compressor.MaybeCompress(hugeMessages)
	require.NotNil(t, res)
	assert.Less(t, res.AfterTokens, res.BeforeTokens)
}
