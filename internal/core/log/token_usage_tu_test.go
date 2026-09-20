package log_test

import (
	"testing"

	"github.com/scandrix/backend/internal/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveAreaMappings(t *testing.T) {
	assert.Equal(t, log.AreaSystem, log.DeriveArea("selectReviewMode", ""))
	assert.Equal(t, log.AreaDrixyRules, log.DeriveArea("drixyRulesAnalyzeCodeWithAI", ""))
	assert.Equal(t, log.AreaDrixyRules, log.DeriveArea("scandrix-rules-review-agent", ""))
	assert.Equal(t, log.AreaReview, log.DeriveArea("code-review-main", ""))
	assert.Equal(t, log.AreaSuggestions, log.DeriveArea("severity-classifier", ""))
	assert.Equal(t, log.AreaSummary, log.DeriveArea("generateSummaryPR", ""))
	assert.Equal(t, log.AreaConversation, log.DeriveArea("conversationAgent", ""))
	assert.Equal(t, log.AreaOther, log.DeriveArea("unrecognizedAgent", ""))
}

func TestDeriveTuComputation(t *testing.T) {
	attrs := map[string]any{
		"gen_ai.usage.total_tokens":                1500,
		"gen_ai.usage.input_tokens":                1000,
		"gen_ai.usage.output_tokens":               500,
		"gen_ai.usage.reasoning_tokens":            120,
		"gen_ai.usage.cache_read_input_tokens":     400,
		"gen_ai.usage.cache_creation_input_tokens": 100,
		"gen_ai.response.model":                    "anthropic:claude-3-5-sonnet",
		"gen_ai.run.name":                          "code-review-executor",
		"type":                                     "byok",
		"credentialId":                             "cred_enterprise_01",
	}

	tu := log.DeriveTu(attrs)
	require.NotNil(t, tu)

	assert.True(t, tu.IsByok)
	assert.False(t, tu.Sys)
	assert.Equal(t, "claude-3-5-sonnet", tu.Model)
	assert.Equal(t, "cred_enterprise_01", tu.CredentialID)
	assert.Equal(t, int64(1500), tu.Total)
	assert.Equal(t, int64(1000), tu.Input)
	assert.Equal(t, int64(500), tu.Output)
	assert.Equal(t, int64(120), tu.Reasoning)
	assert.Equal(t, int64(400), tu.CacheRead)
	assert.Equal(t, int64(100), tu.CacheWrite)
	assert.Equal(t, log.AreaReview, tu.Area)
	assert.Equal(t, "codeReview", tu.Route)
}

func TestDeriveTuNilWhenNoTokens(t *testing.T) {
	assert.Nil(t, log.DeriveTu(nil))
	assert.Nil(t, log.DeriveTu(map[string]any{"gen_ai.usage.total_tokens": 0}))
}
