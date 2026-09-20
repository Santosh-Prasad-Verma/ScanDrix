// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package routing_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/routing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticTaskStrategy(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{ID: "cred-anthropic", Provider: "anthropic", APIKey: "enc-key-1"},
			{ID: "cred-openai", Provider: "openai", APIKey: "enc-key-2"},
		},
		Models: []byok.BYOKModelConfig{
			{ID: "m-sonnet", CredentialID: "cred-anthropic", Model: "claude-sonnet-4.5"},
			{ID: "m-gpt4o", CredentialID: "cred-openai", Model: "gpt-4o"},
			{ID: "m-mini", CredentialID: "cred-openai", Model: "gpt-4o-mini"},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID:  "m-sonnet",
			FallbackModelID: "m-gpt4o",
			TaskOverrides: map[byok.LlmTask]string{
				byok.TaskPRSummary: "m-mini",
			},
		},
	}

	strat := routing.NewStaticTaskStrategy()

	// 1. Default routing for CodeReview
	verdict := strat.Resolve(byok.TaskCodeReview, routing.RequestContext{}, cfg)
	assert.Equal(t, "m-sonnet", verdict.ModelID)
	assert.False(t, verdict.UsedFallback)

	// 2. Task override for PRSummary
	verdict = strat.Resolve(byok.TaskPRSummary, routing.RequestContext{}, cfg)
	assert.Equal(t, "m-mini", verdict.ModelID)

	// 3. Context override wins
	verdict = strat.Resolve(byok.TaskCodeReview, routing.RequestContext{OverrideModelID: "m-gpt4o"}, cfg)
	assert.Equal(t, "m-gpt4o", verdict.ModelID)

	// 4. ResolveTaskSlot with attached fallback
	slot, usedFB, _ := byok.ResolveTaskSlot(cfg, byok.TaskCodeReview, "", "")
	require.NotNil(t, slot)
	assert.Equal(t, "claude-sonnet-4.5", slot.Model)
	assert.False(t, usedFB)
	require.NotNil(t, slot.Fallback)
	assert.Equal(t, "gpt-4o", slot.Fallback.Model)
	assert.True(t, slot.Fallback.UsedFallback)

	// 5. W1: Legacy model name override (not a model ID)
	verdict = strat.Resolve(byok.TaskCodeReview, routing.RequestContext{OverrideModelID: "custom-legacy-model"}, cfg)
	assert.Equal(t, "m-sonnet", verdict.ModelID)
	assert.Equal(t, "custom-legacy-model", verdict.ModelName)

	// 6. Flat inheritance: TaskDrixyRulesReview inherits default model (m-sonnet), not TaskPRSummary override
	verdict = strat.Resolve(byok.TaskDrixyRulesReview, routing.RequestContext{}, cfg)
	assert.Equal(t, "m-sonnet", verdict.ModelID)
}

