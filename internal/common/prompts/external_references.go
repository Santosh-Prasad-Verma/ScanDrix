package prompts

import (
	"fmt"
	"strings"
)

// LineRange specifies a start and end line for a targeted file reference.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// DetectedExternalReference describes a file reference found in natural language or annotations.
type DetectedExternalReference struct {
	FileName       string     `json:"fileName"`
	FilePattern    string     `json:"filePattern,omitempty"`
	Description    string     `json:"description,omitempty"`
	RepositoryName string     `json:"repositoryName,omitempty"`
	OriginalText   string     `json:"originalText,omitempty"`
	LineRange      *LineRange `json:"lineRange,omitempty"`
}

// ExternalReferencesDetectionResponse is the schema returned by external reference detection.
type ExternalReferencesDetectionResponse struct {
	References []DetectedExternalReference `json:"references"`
}

// DetectExternalReferencesSystemPrompt builds the system prompt for discovering external file citations.
func DetectExternalReferencesSystemPrompt() string {
	return `You are an expert at analyzing text to identify file references that require reading external content.

## Core Principle

A file reference exists when the text mentions a file whose CONTENT needs to be read to understand or apply the instructions.

## Two Types of File Mentions

**STRUCTURAL Mentions (DO NOT DETECT):**
References to code structure, imports, or file organization that don't need content.
Example: "Import UserService from services/user.ts" → DON'T DETECT (just an import path)

**CONTENT Mentions (DETECT):**
References where understanding requires reading the actual file content.
Examples:
- "Follow the guidelines in CONTRIBUTING.md" → DETECT (need to read guidelines)
- "Use patterns from docs/api-standards.md" → DETECT (need to read patterns)
- "Validate against schema.json" → DETECT (need to read schema)
- "Check rules in .eslintrc" → DETECT (need to read rules)
- "Follow @file:CONTRIBUTING.md" → DETECT (explicit reference)
- "Use [[file:docs/style.md]]" → DETECT (explicit reference)

## Detection Rules

1. Focus on intent: Does the text require reading the file's content?
2. Support multiple formats:
   - Natural language: "follow guidelines in FILE"
   - Explicit format: "@file:path" or "[[file:path]]"
   - With line ranges: "@file:path#L10-L50" or "[[file:path#L10-L50]]"
   - Cross-repo: "@file:repo-name:path" or "[[file:repo-name:path]]"
3. Be language-agnostic
4. If uncertain, do NOT detect (avoid false positives)
5. Extract line ranges when mentioned (e.g., #L10-L50 means lines 10 to 50)

## What to Extract

For each file requiring content:
- fileName: the file name or path
- filePattern: glob pattern if multiple files referenced
- description: what the file provides (guidelines, rules, examples, etc)
- repositoryName: repository name if explicitly mentioned
- originalText: the EXACT text from the input that mentions this file (for UI highlighting)
- lineRange: specific line range if mentioned (e.g., #L10-L50)

Output format:
{
  "references": [
    {
      "fileName": "file-name.ext",
      "filePattern": "optional-pattern",
      "description": "what content provides",
      "repositoryName": "optional-repo",
      "originalText": "the exact mention from input text",
      "lineRange": { "start": 10, "end": 50 }
    }
  ]
}

Output ONLY valid JSON. No explanations.`
}

// DetectExternalReferencesUserPrompt renders the text and optional context hint for analysis.
func DetectExternalReferencesUserPrompt(text string, contextHint string) string {
	hint := ""
	if contextHint != "" {
		hint = fmt.Sprintf("\n\nContext: This is a %s that may reference external files.", contextHint)
	}

	return fmt.Sprintf("Text to analyze:\n\n%s%s\n\nAnalyze if this text requires external file references.\nReturn JSON with detected file references or empty array if none exist.", text, hint)
}

// RuleExternalReferencesPrompt constructs a prompt to resolve file dependencies across Drixy rules.
func RuleExternalReferencesPrompt(ruleContent, repoContext string) string {
	var sb strings.Builder
	sb.WriteString("Analyze the following Drixy review rule and identify any external configuration or documentation files referenced:\n\n")
	sb.WriteString("Rule Content:\n")
	sb.WriteString(ruleContent)
	if repoContext != "" {
		sb.WriteString("\n\nRepository Context:\n")
		sb.WriteString(repoContext)
	}
	sb.WriteString("\n\nReturn JSON array of required external files with their path and purpose.")
	return sb.String()
}
