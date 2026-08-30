package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSONRepairExecutor coordinates validation and automated 1-shot recovery for LLM responses.
type JSONRepairExecutor struct {
	maxAttempts int
}

// NewJSONRepairExecutor initializes the repair executor.
func NewJSONRepairExecutor(maxAttempts int) *JSONRepairExecutor {
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	return &JSONRepairExecutor{maxAttempts: maxAttempts}
}

// CleanJSONResponse strips markdown code fences, backticks, and extraneous prefix/suffix text.
func CleanJSONResponse(raw string) string {
	cleaned := strings.TrimSpace(raw)

	// Strip ```json ... ``` or ``` ... ```
	if strings.HasPrefix(cleaned, "```") {
		// Remove first line
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		// Remove trailing ```
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
	}

	return strings.TrimSpace(cleaned)
}

// ValidateAndUnmarshal attempts to clean and unmarshal raw LLM output into target struct.
func ValidateAndUnmarshal(raw string, target any) error {
	cleaned := CleanJSONResponse(raw)
	if err := json.Unmarshal([]byte(cleaned), target); err != nil {
		return fmt.Errorf("JSON parse error: %w (raw output snippet: %s)", err, truncateSnippet(cleaned, 120))
	}
	return nil
}

// BuildRepairPrompt creates a corrective prompt for the LLM highlighting the JSON failure.
func BuildRepairPrompt(originalResponse string, parseErr error) string {
	return fmt.Sprintf("Your previous response failed JSON schema parsing with error:\n%s\n\n"+
		"Please fix the syntax and return ONLY a valid, raw JSON object matching the required schema without markdown fences, explanation, or commentary.",
		parseErr.Error(),
	)
}

// AttachAnthropicPromptCache adds ephemeral cache control metadata for large static prompt blocks.
func AttachAnthropicPromptCache(systemPrompt string) map[string]any {
	return map[string]any{
		"type": "text",
		"text": systemPrompt,
		"cache_control": map[string]string{
			"type": "ephemeral",
		},
	}
}

func truncateSnippet(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
