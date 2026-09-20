// Package agentcore provides the deep core agent loop and execution components for code reviews.
package agentcore

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// FallbackCharsPerToken is calibrated for dense code, AST markup, and multi-file unified diffs (2.8 chars/token).
const FallbackCharsPerToken = 2.8

// CharsPerToken returns the configured char-to-token ratio, checking environment overrides.
func CharsPerToken() float64 {
	if v := os.Getenv("SCANDRIX_CHARS_PER_TOKEN"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return FallbackCharsPerToken
}

// EstimateTokens calculates an approximate token count for plain text or code strings.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	ratio := CharsPerToken()
	tokens := int(math.Ceil(float64(len(text)) / ratio))
	if tokens < 1 {
		return 1
	}
	return tokens
}

// TokensToChars converts a token count back into an approximate character count.
func TokensToChars(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	return int(math.Floor(float64(tokens) * CharsPerToken()))
}

// EstimateValueTokens estimates token count of an arbitrary structured object or map
// by JSON-serializing it the same way the model provider receives it on the wire.
func EstimateValueTokens(value any) int {
	if value == nil {
		return 0
	}
	if str, ok := value.(string); ok {
		return EstimateTokens(str)
	}

	b, err := json.Marshal(value)
	if err != nil {
		return EstimateTokens(fmt.Sprintf("%v", value))
	}
	return EstimateTokens(string(b))
}

// EstimateOverheadTokens counts fixed per-request overhead (system prompt + tool schemas)
// that the model provider re-sends on every turn.
func EstimateOverheadTokens(systemPrompt string, toolDefs []any) int {
	total := EstimateTokens(systemPrompt)
	for _, def := range toolDefs {
		total += EstimateValueTokens(def)
	}
	return total
}

// EstimateAgentSpecOverhead calculates total fixed per-turn token overhead for an AgentSpec.
func EstimateAgentSpecOverhead(spec contracts.AgentSpec) int {
	var toolDefs []any
	if spec.Tools != nil {
		for _, t := range spec.Tools.List() {
			toolDefs = append(toolDefs, map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.InputSchema(),
			})
		}
	}
	return EstimateOverheadTokens(spec.SystemPrompt, toolDefs)
}
