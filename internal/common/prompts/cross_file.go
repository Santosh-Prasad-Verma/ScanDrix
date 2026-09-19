// Package prompts implements cross-file analysis prompts for PR reviews in ScanDrix.
package prompts

import (
	"fmt"
	"strings"
)

// CrossFileAnalysisPayload contains input files, overrides, and cross-file dependencies.
type CrossFileAnalysisPayload struct {
	Files              []ChangedFile      `json:"files"`
	Language           string             `json:"language"`
	Overrides          PromptOverrides    `json:"overrides"`
	CrossFileContexts  []CrossFileContext `json:"crossFileContexts,omitempty"`
	Memories           []MemoryItem       `json:"memories,omitempty"`
	ExternalReferences []string           `json:"externalReferences,omitempty"`
}

// PromptCodeReviewCrossFileAnalysis generates the cross-file review prompt for multi-file PR analysis.
func PromptCodeReviewCrossFileAnalysis(payload CrossFileAnalysisPayload) string {
	lang := payload.Language
	if lang == "" {
		lang = "en-US"
	}

	criticalText := payload.Overrides.Critical
	if criticalText == "" {
		criticalText = "- Critical security vulnerabilities, data corruption, auth bypass, severe regressions."
	}
	highText := payload.Overrides.High
	if highText == "" {
		highText = "- Breaking interface changes, unhandled panics/exceptions, major logic flaws."
	}
	mediumText := payload.Overrides.Medium
	if mediumText == "" {
		mediumText = "- Significant code duplication, missing error handling, performance anti-patterns."
	}
	lowText := payload.Overrides.Low
	if lowText == "" {
		lowText = "- Naming inconsistencies, code organization, minor readability suggestions."
	}

	mainGen := payload.Overrides.MainGen
	if mainGen == "" {
		mainGen = "Provide clear, concise explanation of the cross-file issue with exact references."
	}

	var contextSB strings.Builder
	if len(payload.CrossFileContexts) > 0 {
		contextSB.WriteString("## Retrieved Cross-File Context\n\nCode snippets retrieved outside this PR that consume or depend on changed code:\n\n")
		for _, ctx := range payload.CrossFileContexts {
			contextSB.WriteString(fmt.Sprintf("### %s\n**Relationship:** %s\n**Rationale:** %s\n\n```\n%s\n```\n\n",
				ctx.FilePath, ctx.Relationship, ctx.Rationale, ctx.Content))
		}
	}

	basePrompt := fmt.Sprintf(`You are Drixy PR-Reviewer, an expert senior engineer specialized in multi-file architectural review. You are context-aware and prioritize developer intent over rigid rule-following.

# Cross-File Code Analysis
Analyze the following PR files for patterns that require multiple file context: duplicate implementations, inconsistent error handling, configuration drift, interface inconsistencies, and redundant operations.

## Input Files
%s

%s
## Analysis Focus
- Same logic implemented across multiple files in the diff
- Inconsistent error propagation and validation rules between components
- Hardcoded constants duplicated across files that should be shared
- Unnecessary database or network calls when data is already available
- Broken interface contracts or changed signatures impacting external callers

## Suppression Criteria (MANDATORY)
1. **Documented Intent / Tech Debt**: Comments like TODO, FIXME, HACK, or explicit deprecation.
2. **Testing & Mocks**: Test files, fixtures, or mocks with intentional hardcoded values.
3. **Auto-Generated Code**: Files marked DO NOT EDIT or protobuf generated.
4. **Feature Flags**: Temporary branching behind progressive rollout flags.

## Severity Assessment
**CRITICAL**: %s
**HIGH**: %s
**MEDIUM**: %s
**LOW**: %s

## Output Format (Strict JSON)
Generate suggestions strictly in valid JSON format:
{
  "suggestions": [
    {
      "relevantFile": "primary affected file",
      "relatedFile": "secondary affected file",
      "language": "detected language",
      "suggestionContent": "%s",
      "existingCode": "original code block",
      "improvedCode": "refactored clean code",
      "oneSentenceSummary": "short summary",
      "relevantLinesStart": 1,
      "relevantLinesEnd": 10,
      "severity": "low | medium | high | critical",
      "llmPrompt": "User prompt for chat with LLM"
    }
  ]
}

- Language: Output in %s
- Current Date: %s`,
		ToJSONString(payload.Files),
		contextSB.String(),
		criticalText, highText, mediumText, lowText,
		mainGen,
		lang,
		CurrentDateString(),
	)

	var extraSections []string
	if mem := FormatMemoriesSection(payload.Memories); mem != "" {
		extraSections = append(extraSections, mem)
	}
	if len(payload.ExternalReferences) > 0 {
		extraSections = append(extraSections, strings.Join(payload.ExternalReferences, "\n\n"))
	}

	return FormatExternalContext(basePrompt, extraSections)
}

// PromptCrossFileContextPlanner plans which external repository files need to be retrieved for context.
func PromptCrossFileContextPlanner(files []ChangedFile, language string) string {
	return fmt.Sprintf(`You are Drixy Context Planner. Identify external files in the repository that are directly impacted by or depend on the changed symbols in this PR.

## PR Changed Files
%s

Output a JSON array of file paths to inspect:
{
  "targetFiles": ["path/to/caller.go", "path/to/interface.go"],
  "rationale": "Verify callers of modified signatures"
}`, ToJSONString(files))
}

// PromptCrossFileContextSufficiency verifies whether the retrieved external context is sufficient.
func PromptCrossFileContextSufficiency(contexts []CrossFileContext, prDiff string) string {
	return fmt.Sprintf(`You are Drixy Context Validator. Determine if the retrieved files provide sufficient evidence to confirm or refute cross-file breakage.

## Retrieved Context
%s

## PR Diff
%s

Output JSON: {"isSufficient": true, "missingContext": ""}`, ToJSONString(contexts), prDiff)
}
