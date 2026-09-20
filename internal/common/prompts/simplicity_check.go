package prompts

import (
	"fmt"
)

// CheckSuggestionSimplicityResponse represents the judgment whether a code change is simple and self-contained.
type CheckSuggestionSimplicityResponse struct {
	IsSimple bool   `json:"isSimple"`
	Reason   string `json:"reason,omitempty"`
}

// SimplicityCheckPayload contains the original and improved code snippets for comparison.
type SimplicityCheckPayload struct {
	Language     string `json:"language"`
	ExistingCode string `json:"existingCode"`
	ImprovedCode string `json:"improvedCode"`
}

// PromptCheckSuggestionSimplicitySystem returns the system instructions for verifying suggestion simplicity.
func PromptCheckSuggestionSimplicitySystem() string {
	return `You are an expert code reviewer. Your task is to analyze a code suggestion and determine if it is "simple" and safe to apply without needing to see other files.

A suggestion is considered **COMPLEX** (unsafe) if:
- It likely requires changes in other files (e.g., changing a function signature used elsewhere).
- It introduces new imports that might be missing or conflict.
- It changes the behavior in a way that requires understanding the broader system architecture.
- It is a large refactoring.
- It is not a contiguous block of code (e.g., changes scattered across multiple line ranges).

A suggestion is considered **SIMPLE** (safe) if:
- It is a local change (e.g., renaming a local variable, fixing a typo, small logic fix within a function).
- It relies only on existing imports or standard library imports.
- It is self-contained within the provided code block.

Respond with a JSON object:
{
    "isSimple": boolean,
    "reason": "Short explanation of why it is simple or complex"
}

Analyze the following suggestion:`
}

// PromptCheckSuggestionSimplicityUser formats the original and improved code for simplicity analysis.
func PromptCheckSuggestionSimplicityUser(payload SimplicityCheckPayload) string {
	lang := payload.Language
	if lang == "" {
		lang = "text"
	}

	return fmt.Sprintf(`
Original Code:
`+"```%s\n%s\n```"+`

Improved Code:
`+"```%s\n%s\n```", lang, payload.ExistingCode, lang, payload.ImprovedCode)
}
