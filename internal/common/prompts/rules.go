// Package prompts implements custom rule evaluation, rule generation, and rule recommendations for ScanDrix.
package prompts

import (
	"fmt"
	"strings"
)

// PromptDrixyRules evaluates PR changed files against custom active repository rules.
func PromptDrixyRules(files []ChangedFile, activeRules []RuleItem) string {
	return fmt.Sprintf(`You are Drixy Rule Auditor. Evaluate the pull request diff against the active custom review rules.

## Active Rules
%s

## PR Files
%s

## Instructions
For each rule, examine whether any diff violates the rule's specific guidelines.
Only report clear, unambiguous violations backed by evidence.
Output JSON:
{
  "ruleViolations": [
    {
      "ruleTitle": "title of rule",
      "file": "path/to/file",
      "lineStart": 1,
      "lineEnd": 5,
      "severity": "high",
      "explanation": "why this violates the rule",
      "suggestedFix": "corrected code snippet"
    }
  ]
}`, ToJSONString(activeRules), ToJSONString(files))
}

// PromptDrixyRulesGenerator synthesizes a structured custom rule from a code snippet and description.
func PromptDrixyRulesGenerator(codeSnippet, description string) string {
	return fmt.Sprintf(`You are Drixy Rule Synthesizer. Convert the following engineering guideline or anti-pattern into a structured ScanDrix Rule file.

## Guideline Description
%s

## Example Snippet
%s

Generate a rule file matching the format:
---
title: Concise Imperative Rule Title
severity_min: high
scope: file
path: "**/*"
enabled: true
---

Clear explanation of the rule, why it exists, and technical context.

### Bad Example
`+"```go\n// anti-pattern snippet\n```"+`

### Good Example
`+"```go\n// corrected pattern snippet\n```", description, codeSnippet)
}

// PromptDrixyRulesPRLevel analyzes whether a PR as a whole satisfies team architectural conventions.
func PromptDrixyRulesPRLevel(prTitle, prBody, diff string) string {
	return fmt.Sprintf(`Evaluate pull request metadata and changes for high-level architectural conventions:
Title: %s
Description: %s

Diff:
%s

Output JSON: {"isCompliant": true, "suggestions": []}`, prTitle, prBody, diff)
}

// PromptDrixyRulesRecommendation recommends new custom rules based on recurring issues in PR diffs.
func PromptDrixyRulesRecommendation(diff string, currentRules []string) string {
	return fmt.Sprintf(`Identify recurring code smells or anti-patterns in this diff that warrant codifying into new reusable team rules.

## Existing Active Rules
%s

## PR Diff
%s

Output JSON:
{
  "recommendations": [
    {
      "proposedTitle": "Rule Title",
      "rationale": "Why this should be a team-wide rule",
      "severity": "medium"
    }
  ]
}`, strings.Join(currentRules, "\n"), diff)
}

// PromptDrixyMemoryResolution resolves memories and learnings from past reviews.
func PromptDrixyMemoryResolution(memories []MemoryItem, query string) string {
	return fmt.Sprintf(`Given the user query and past review memories, resolve relevant architectural conventions:
Query: %s

Memories:
%s

Output JSON: {"relevantMemories": []}`, query, ToJSONString(memories))
}
