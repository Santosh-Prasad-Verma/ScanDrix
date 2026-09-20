// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contextwin_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/llm/contextwin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelContextWindow(t *testing.T) {
	// Manual overrides
	assert.Equal(t, 200_000, contextwin.GetModelContextWindow("claude-sonnet-4.5"))
	assert.Equal(t, 1_048_576, contextwin.GetModelContextWindow("gemini-2.5-pro"))
	assert.Equal(t, 1_000_000, contextwin.GetModelContextWindow("gpt-5.4"))
	assert.Equal(t, 262_144, contextwin.GetModelContextWindow("kimi-k2.7"))

	// Exact / Normalized / Substring in DB
	gpt4o := contextwin.GetModelContextWindow("gpt-4o")
	assert.Equal(t, 128_000, gpt4o)

	// Fallback
	unknown := contextwin.GetModelContextWindow("some-completely-unknown-model-xyz")
	assert.Equal(t, 128_000, unknown)

	// Explicit override
	assert.Equal(t, 64_000, contextwin.ResolveContextWindow(64_000, "gpt-4o"))
}

func TestAssertPromptFitsInContext(t *testing.T) {
	// Fits
	err := contextwin.AssertPromptFitsInContext("System prompt", "User code diff", 128_000, "gpt-4o")
	require.NoError(t, err)

	// Exceeds
	hugeUserPrompt := strings.Repeat("abcd", 40_000) // 160k chars => ~40k tokens
	err = contextwin.AssertPromptFitsInContext("System prompt", hugeUserPrompt, 32_000, "claude-small")
	require.Error(t, err)
	var promptErr *contextwin.PromptTooLargeError
	assert.ErrorAs(t, err, &promptErr)
	assert.Equal(t, "claude-small", promptErr.ModelName)
}
