// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm_test

import (
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/llm"
	"github.com/stretchr/testify/assert"
)

func TestIsContextOverflowResult(t *testing.T) {
	state := contracts.RunState{
		Status: contracts.StatusError,
		Trace: []contracts.TraceEvent{
			{
				Kind: "error",
				Detail: map[string]any{
					"message": "context_length_exceeded: This model's maximum context length is 128000 tokens",
				},
			},
		},
	}

	assert.True(t, llm.IsContextOverflowResult(state))

	cleanState := contracts.RunState{
		Status: contracts.StatusCompleted,
	}
	assert.False(t, llm.IsContextOverflowResult(cleanState))

	rateLimitState := contracts.RunState{
		Status: contracts.StatusError,
		Trace: []contracts.TraceEvent{
			{
				Kind: "error",
				Detail: map[string]any{
					"message": "rate limit exceeded: 429 Too Many Requests",
				},
			},
		},
	}
	assert.False(t, llm.IsContextOverflowResult(rateLimitState))
}
