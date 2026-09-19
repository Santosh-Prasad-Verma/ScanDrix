package log_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenUsageStress_ExhaustiveAgentAreaMapping verifies deterministic mapping
// from any known run name or agent phase to the corresponding TokenUsageArea enum.
func TestTokenUsageStress_ExhaustiveAgentAreaMapping(t *testing.T) {
	testCases := []struct {
		runName      string
		phase        string
		expectedArea log.TokenUsageArea
	}{
		// System run names
		{"selectReviewMode", "", log.AreaSystem},
		{"validateImplementedSuggestions", "", log.AreaSystem},
		{"generateCodeSuggestions", "", log.AreaSystem},

		// Drixy Rules
		{"drixyRulesReview", "", log.AreaDrixyRules},
		{"scandrix_rules_agent", "", log.AreaDrixyRules},
		{"drixyMemoryLookup", "", log.AreaDrixyRules},
		{"drixyMemorySync", "", log.AreaDrixyRules},

		// Core Review
		{"code-review-worker", "", log.AreaReview},
		{"analyzeCodeWithAI_pass1", "", log.AreaReview},
		{"analyzeCodeWithAI_ast", "", log.AreaReview},

		// Suggestion runs
		{"severity-classifier", "", log.AreaSuggestions},
		{"suggestion-formatter", "", log.AreaSuggestions},
		{"severityAnalysis", "", log.AreaSuggestions},
		{"validateWithLLM", "", log.AreaSuggestions},
		{"checkSuggestionSimplicity", "", log.AreaSuggestions},
		{"repeatedCodeReviewSuggestionClustering", "", log.AreaSuggestions},
		{"safeguard-filter", "", log.AreaSuggestions},

		// Summary
		{"generateSummaryPR", "", log.AreaSummary},
		{"generateSummaryPR_v2", "", log.AreaSummary},

		// Conversation
		{"conversationAgent", "", log.AreaConversation},
		{"anyOtherAgent", "conversation", log.AreaConversation},

		// Unrecognized / Other
		{"unknownTaskAgent", "", log.AreaOther},
		{"customUserScript", "processing", log.AreaOther},
		{"", "", log.AreaOther},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s_%s", tc.runName, tc.phase), func(t *testing.T) {
			got := log.DeriveArea(tc.runName, tc.phase)
			assert.Equal(t, tc.expectedArea, got)
		})
	}
}

// TestTokenUsageStress_RouteDerivationAndOverride ensures route strings adhere
// to the expected routing tasks matrix and override rules.
func TestTokenUsageStress_RouteDerivationAndOverride(t *testing.T) {
	// 1. Inferred from area
	assert.Equal(t, "codeReview", log.RouteFromArea(log.AreaReview))
	assert.Equal(t, "codeReview", log.RouteFromArea(log.AreaSuggestions))
	assert.Equal(t, "drixyRulesReview", log.RouteFromArea(log.AreaDrixyRules))
	assert.Equal(t, "prSummary", log.RouteFromArea(log.AreaSummary))
	assert.Equal(t, "conversation", log.RouteFromArea(log.AreaConversation))
	assert.Equal(t, "", log.RouteFromArea(log.AreaSystem))
	assert.Equal(t, "", log.RouteFromArea(log.AreaOther))

	// 2. Explicit valid route preserved in DeriveTu
	attrsWithValidRoute := map[string]any{
		"gen_ai.usage.total_tokens": 100,
		"gen_ai.run.name":           "code-review-main",
		"route":                     "businessValidation", // recognized routing task
	}
	tu := log.DeriveTu(attrsWithValidRoute)
	require.NotNil(t, tu)
	assert.Equal(t, "businessValidation", tu.Route)

	// 3. Unrecognized route falls back to RouteFromArea
	attrsWithBogusRoute := map[string]any{
		"gen_ai.usage.total_tokens": 100,
		"gen_ai.run.name":           "generateSummaryPR",
		"route":                     "bogus_custom_route",
	}
	tu2 := log.DeriveTu(attrsWithBogusRoute)
	require.NotNil(t, tu2)
	assert.Equal(t, "prSummary", tu2.Route)
}

// TestTokenUsageStress_CanonicalModelIDPrefixStripping verifies provider prefix removal.
func TestTokenUsageStress_CanonicalModelIDPrefixStripping(t *testing.T) {
	cases := []struct {
		raw      string
		expected string
	}{
		{"anthropic:claude-3-5-sonnet", "claude-3-5-sonnet"},
		{"anthropic:claude-3-opus", "claude-3-opus"},
		{"google:gemini-1.5-pro", "gemini-1.5-pro"},
		{"google:gemini-2.0-flash", "gemini-2.0-flash"},
		{"openai:gpt-4o", "gpt-4o"},
		{"openai:o1-preview", "o1-preview"},
		{"deepseek-chat", "deepseek-chat"},
		{"mistral-large-2407", "mistral-large-2407"},
		{"", ""},
	}

	for _, tc := range cases {
		got := log.CanonicalModelID(tc.raw)
		assert.Equal(t, tc.expected, got)
	}
}

// TestTokenUsageStress_ConcurrencyAndThreadSafety processes thousands of DeriveTu
// calls concurrently across 40 goroutines with zero data races.
func TestTokenUsageStress_ConcurrencyAndThreadSafety(t *testing.T) {
	const numWorkers = 40
	const iterations = 250

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				attrs := map[string]any{
					"gen_ai.usage.total_tokens":                int64(1000 + i),
					"gen_ai.usage.input_tokens":                int64(700 + i),
					"gen_ai.usage.output_tokens":               int64(300),
					"gen_ai.usage.reasoning_tokens":            int64(50),
					"gen_ai.usage.cache_read_input_tokens":     int64(200),
					"gen_ai.usage.cache_creation_input_tokens": int64(100),
					"gen_ai.response.model":                    "anthropic:claude-3-5-sonnet",
					"gen_ai.run.name":                          "code-review-subagent",
					"type":                                     "byok",
					"credentialId":                             fmt.Sprintf("cred_%d", workerID),
				}

				tu := log.DeriveTu(attrs)
				assert.NotNil(t, tu)
				assert.Equal(t, int64(1000+i), tu.Total)
				assert.Equal(t, "claude-3-5-sonnet", tu.Model)
				assert.Equal(t, log.AreaReview, tu.Area)
			}
		}(w)
	}

	wg.Wait()
}

// TestTokenUsageStress_JSONSerializationContract validates that serialized TokenUsageTu
// contains the exact camelCase properties specified by the ScanDrix indexer contract.
func TestTokenUsageStress_JSONSerializationContract(t *testing.T) {
	tu := &log.TokenUsageTu{
		IsByok:       true,
		Sys:          false,
		Model:        "gpt-4o",
		CredentialID: "cred_prod_001",
		Input:        500,
		Output:       200,
		Total:        700,
		Reasoning:    45,
		CacheRead:    100,
		CacheWrite:   50,
		Area:         log.AreaReview,
		Route:        "codeReview",
	}

	bytes, err := json.Marshal(tu)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(bytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, true, parsed["isByok"])
	assert.Equal(t, false, parsed["sys"])
	assert.Equal(t, "gpt-4o", parsed["model"])
	assert.Equal(t, "cred_prod_001", parsed["credentialId"])
	assert.Equal(t, float64(500), parsed["input"])
	assert.Equal(t, float64(200), parsed["output"])
	assert.Equal(t, float64(700), parsed["total"])
	assert.Equal(t, float64(45), parsed["reasoning"])
	assert.Equal(t, float64(100), parsed["cacheRead"])
	assert.Equal(t, float64(50), parsed["cacheWrite"])
	assert.Equal(t, "review", parsed["area"])
	assert.Equal(t, "codeReview", parsed["route"])
}
