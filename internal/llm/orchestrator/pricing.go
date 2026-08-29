package orchestrator

import (
	"math"
)

var defaultCatalog = map[string]ModelProfile{
	"claude-3-7-sonnet": {
		ModelID:          "claude-3-7-sonnet",
		Provider:         ProviderAnthropic,
		ContextWindow:    200000,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: true,
	},
	"claude-3-5-sonnet": {
		ModelID:          "claude-3-5-sonnet",
		Provider:         ProviderAnthropic,
		ContextWindow:    200000,
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		SupportsThinking: false,
	},
	"gpt-4o": {
		ModelID:          "gpt-4o",
		Provider:         ProviderOpenAI,
		ContextWindow:    128000,
		InputPerMillion:  2.50,
		OutputPerMillion: 10.00,
		SupportsThinking: false,
	},
	"gemini-2.5-pro": {
		ModelID:          "gemini-2.5-pro",
		Provider:         ProviderGemini,
		ContextWindow:    1000000,
		InputPerMillion:  1.25,
		OutputPerMillion: 5.00,
		SupportsThinking: true,
	},
	"novita-deepseek-r1": {
		ModelID:          "novita-deepseek-r1",
		Provider:         ProviderNovita,
		ContextWindow:    64000,
		InputPerMillion:  0.55,
		OutputPerMillion: 2.19,
		SupportsThinking: true,
	},
}

// GetModelProfile returns the technical specifications and pricing for a model.
func GetModelProfile(modelID string) (ModelProfile, bool) {
	p, ok := defaultCatalog[modelID]
	return p, ok
}

// CalculateCost computes the exact USD cost for an inference call with 6 decimal precision.
func CalculateCost(modelID string, promptTokens, completionTokens int) float64 {
	profile, ok := defaultCatalog[modelID]
	if !ok {
		// Fallback default estimation ($2.50 / $10.00)
		profile = defaultCatalog["gpt-4o"]
	}

	inputCost := (float64(promptTokens) / 1_000_000.0) * profile.InputPerMillion
	outputCost := (float64(completionTokens) / 1_000_000.0) * profile.OutputPerMillion

	total := inputCost + outputCost
	// Round to 6 decimal places
	return math.Round(total*1_000_000) / 1_000_000
}
