// Package prompts implements suggestion analysis, clustering, and comment analysis prompts for ScanDrix.
package prompts

import (
	"fmt"
)

// PromptCheckSuggestionSimplicity verifies if a suggestion is concise and focused.
func PromptCheckSuggestionSimplicity(suggestionContent, diff string) string {
	return fmt.Sprintf(`Evaluate if this code review suggestion is simple, focused, and directly actionable:
Suggestion: %s

Diff Context:
%s

Output JSON: {"isSimple": true, "suggestedRevision": ""}`, suggestionContent, diff)
}

// PromptRepeatedCodeReviewSuggestionClustering clusters duplicate or related suggestions together.
func PromptRepeatedCodeReviewSuggestionClustering(suggestions []SuggestionItem) string {
	return fmt.Sprintf(`Cluster related or duplicate suggestions to reduce reviewer noise and present a unified fix:
Suggestions:
%s

Output JSON:
{
  "clusters": [
    {
      "rootCause": "Shared issue",
      "files": ["file1", "file2"],
      "consolidatedSuggestion": "Unified fix"
    }
  ]
}`, ToJSONString(suggestions))
}

// PromptSeverityAnalysis calculates calibrated severity for an identified code issue.
func PromptSeverityAnalysis(suggestion, diff string) string {
	return fmt.Sprintf(`Analyze the actual real-world risk and blast radius of this issue:
Issue: %s

Diff Context:
%s

Output JSON:
{
  "severity": "low | medium | high | critical",
  "reasoning": "rationale for rating"
}`, suggestion, diff)
}

// PromptValidateCodeSemantics verifies that the improved code preserves correct semantics and does not break callers.
func PromptValidateCodeSemantics(suggestion, originalCode, improvedCode string) string {
	return fmt.Sprintf(`Verify that proposed replacement code is strictly equivalent in semantics and free of runtime regressions:
Original Code:
%s

Improved Code:
%s

Review Suggestion:
%s

Output JSON:
{
  "isSemanticallyCorrect": true | false,
  "detectedRegressions": ""
}`, originalCode, improvedCode, suggestion)
}

// PromptCommentAnalysis analyzes developer comments on review suggestions to steer subsequent revisions.
func PromptCommentAnalysis(comment, diff string) string {
	return fmt.Sprintf(`Analyze reviewer feedback on an automated review finding:
Comment: %s

Context Diff:
%s

Output JSON:
{
  "userIntent": "accept | dispute | request_clarification",
  "actionableSteps": "how to respond"
}`, comment, diff)
}
