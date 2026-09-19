// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package structured_test

import (
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/llm/structured"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sampleReview struct {
	Verdict string   `json:"verdict"`
	Score   int      `json:"score"`
	Files   []string `json:"files,omitempty"`
}

func TestSliceBalancedJSON(t *testing.T) {
	// Nested with string quotes containing braces
	raw := `Here is the review result: {"verdict": "pass {with notes}", "score": 95} Hope it helps!`
	balanced := structured.SliceBalancedJSON(raw)
	assert.Equal(t, `{"verdict": "pass {with notes}", "score": 95}`, balanced)

	// Array format
	rawArr := `Prefix [1, 2, {"a": "b"}] Suffix`
	assert.Equal(t, `[1, 2, {"a": "b"}]`, structured.SliceBalancedJSON(rawArr))

	// No JSON
	assert.Equal(t, "", structured.SliceBalancedJSON("plain text without delimiters"))
}

func TestExtractJSONFromText(t *testing.T) {
	// Markdown fenced with trailing comma
	raw := "```json\n{\n  \"verdict\": \"clean\",\n  \"score\": 100,\n}\n```"
	extracted := structured.ExtractJSONFromText(raw)
	assert.Equal(t, "{\n  \"verdict\": \"clean\",\n  \"score\": 100\n}", extracted)

	// Salvage into struct
	var res sampleReview
	ok := structured.SalvageStructuredError(raw, &res)
	require.True(t, ok)
	assert.Equal(t, "clean", res.Verdict)
	assert.Equal(t, 100, res.Score)
}

func TestRepairJSONText(t *testing.T) {
	// Trailing comma
	badJSON := `{"verdict": "ok", "score": 80,}`
	repaired := structured.RepairJSONText(badJSON)
	assert.Equal(t, `{"verdict": "ok", "score": 80}`, repaired)

	// Valid JSON unchanged returns empty string (nothing to repair)
	goodJSON := `{"verdict": "ok", "score": 80}`
	assert.Equal(t, "", structured.RepairJSONText(goodJSON))
}

func TestToStrictWireSchema(t *testing.T) {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"verdict": map[string]any{"type": "string"},
			"score":   map[string]any{"type": "integer"},
			"details": map[string]any{"type": "string"},
		},
		"required": []any{"verdict"},
	}

	strict := structured.ToStrictWireSchema(inputSchema)
	assert.False(t, strict["additionalProperties"].(bool))

	req := strict["required"].([]any)
	assert.Len(t, req, 3)

	props := strict["properties"].(map[string]any)
	scoreProp := props["score"].(map[string]any)
	assert.Contains(t, scoreProp, "anyOf")
}

func TestGateAndPlans(t *testing.T) {
	assert.True(t, structured.OpenRouterHonorsJSONSchema("openai/gpt-4o"))
	assert.False(t, structured.OpenRouterHonorsJSONSchema("random/custom-model"))

	assert.True(t, structured.OpenAICompatibleHonorsJSONSchema("http://localhost:8000/v1"))
	assert.True(t, structured.OpenAICompatibleHonorsJSONSchema("https://api.fireworks.ai/inference/v1"))

	assert.True(t, structured.IsNeverDowngradeModel("kimi-k2.7-chat"))
	assert.True(t, structured.IsNeverDowngradeModel("moonshot-v1"))

	// Cache
	assert.True(t, structured.MayUseJSONSchema("openai", "gpt-4o", ""))
	structured.MarkJSONSchemaUnsupported("openai", "gpt-4o", "")
	assert.False(t, structured.MayUseJSONSchema("openai", "gpt-4o", ""))

	// Error check
	err := errors.New("400 Bad Request: response_format json_schema is unsupported for this model")
	assert.True(t, structured.IsJSONSchemaUnsupportedError(err))

	// Plan resolution
	assert.Equal(t, structured.PlanRerouteJSON, structured.ResolveStructuredPlan("moonshot", "kimi-k2.7", true, true, false))
	assert.Equal(t, structured.PlanSuppressThinking, structured.ResolveStructuredPlan("anthropic", "claude-sonnet-4", true, false, true))
	assert.Equal(t, structured.PlanAsIs, structured.ResolveStructuredPlan("openai", "gpt-4o", false, false, false))
}

func TestStripNullProps(t *testing.T) {
	input := map[string]any{
		"a": "keep",
		"b": nil,
		"c": map[string]any{
			"nested_null": nil,
			"nested_keep": 42,
		},
		"arr": []any{
			map[string]any{"x": nil, "y": "ok"},
			"val",
		},
	}

	res := structured.StripNullProps(input).(map[string]any)
	assert.Equal(t, "keep", res["a"])
	_, hasB := res["b"]
	assert.False(t, hasB)

	nested := res["c"].(map[string]any)
	_, hasNestedNull := nested["nested_null"]
	assert.False(t, hasNestedNull)
	assert.Equal(t, 42, nested["nested_keep"])

	arr := res["arr"].([]any)
	arrMap := arr[0].(map[string]any)
	_, hasX := arrMap["x"]
	assert.False(t, hasX)
	assert.Equal(t, "ok", arrMap["y"])
}

