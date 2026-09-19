// Package compression provides context window budgeting and token estimation for agent runs.
package compression

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// FallbackCharsPerToken is calibrated for dense code/markup (2.8 chars per token).
const FallbackCharsPerToken = 2.8

// CharsPerToken returns the configured char-to-token ratio.
func CharsPerToken() float64 {
	if v := os.Getenv("TOKEN_ESTIMATE_CHARS_PER_TOKEN"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return FallbackCharsPerToken
}

// EstimateTextTokens estimates token count using calibrated ratio for dense code.
func EstimateTextTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	ratio := CharsPerToken()
	tokens := int(float64(len(text)) / ratio)
	if tokens < 1 {
		return 1
	}
	return tokens
}

// EstimateValueTokens estimates token count of an arbitrary structured object
// by JSON-serializing it the same way the model provider receives it on the wire.
func EstimateValueTokens(value any) int {
	if value == nil {
		return 0
	}
	if str, ok := value.(string); ok {
		return EstimateTextTokens(str)
	}

	b, err := json.Marshal(value)
	if err != nil {
		return EstimateTextTokens(fmt.Sprintf("%v", value))
	}
	return EstimateTextTokens(string(b))
}

// EstimateOverheadTokens counts fixed per-request overhead (system prompt + tool schemas)
// that the model provider re-sends on every turn.
func EstimateOverheadTokens(systemPrompt string, toolDefs []any) int {
	total := EstimateTextTokens(systemPrompt)
	for _, def := range toolDefs {
		total += EstimateValueTokens(def)
	}
	return total
}
