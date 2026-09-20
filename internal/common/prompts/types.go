// Package prompts provides structured LLM prompts, cross-file review analyzers, safeguard checkers, and rule builders for ScanDrix.
package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ChangedFile represents a changed file and its diff in a pull request.
type ChangedFile struct {
	Filename string `json:"filename"`
	CodeDiff string `json:"codeDiff"`
}

// CrossFileContext represents related context extracted outside the PR diff.
type CrossFileContext struct {
	FilePath      string `json:"filePath"`
	Content       string `json:"content"`
	Rationale     string `json:"rationale"`
	Relationship  string `json:"relationship"`
	RelatedSymbol string `json:"relatedSymbol,omitempty"`
}

// RuleItem represents a rule definition passed into prompt context.
type RuleItem struct {
	Title       string   `json:"title"`
	Rule        string   `json:"rule"`
	Severity    string   `json:"severity"`
	Path        string   `json:"path"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// MemoryItem represents learnings from past code reviews.
type MemoryItem struct {
	ID       string `json:"id,omitempty"`
	Title    string `json:"title"`
	Content  string `json:"content,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Category string `json:"category,omitempty"`
}

// SuggestionItem represents an actionable review finding.
type SuggestionItem struct {
	RelevantFile       string `json:"relevantFile"`
	RelatedFile        string `json:"relatedFile,omitempty"`
	Language           string `json:"language"`
	SuggestionContent  string `json:"suggestionContent"`
	ExistingCode       string `json:"existingCode"`
	ImprovedCode       string `json:"improvedCode"`
	OneSentenceSummary string `json:"oneSentenceSummary"`
	RelevantLinesStart int    `json:"relevantLinesStart"`
	RelevantLinesEnd   int    `json:"relevantLinesEnd"`
	Severity           string `json:"severity"`
	LLMPrompt          string `json:"llmPrompt,omitempty"`
}

// PromptOverrides allows configuring customized reviewer behavior and severity flags.
type PromptOverrides struct {
	Critical string
	High     string
	Medium   string
	Low      string
	MainGen  string
}

// SanitizePromptText strips control characters and trims text.
func SanitizePromptText(text string) string {
	clean := strings.ReplaceAll(text, "\x00", "")
	return strings.TrimSpace(clean)
}

// FormatMemoriesSection formats historical review memory items.
func FormatMemoriesSection(memories []MemoryItem) string {
	if len(memories) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Memories\n\nAdditional context from past learnings in Drixy Rules format.\n\n")
	for _, m := range memories {
		ruleText := m.Content
		if ruleText == "" {
			ruleText = m.Rule
		}
		if m.Title != "" && ruleText != "" {
			sb.WriteString(fmt.Sprintf("- Title: %s\n  Rule: %s\n\n", SanitizePromptText(m.Title), SanitizePromptText(ruleText)))
		}
	}
	return sb.String()
}

// FormatExternalContext appends external context sections to a base prompt.
func FormatExternalContext(basePrompt string, sections []string) string {
	var valid []string
	for _, s := range sections {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			valid = append(valid, trimmed)
		}
	}
	if len(valid) == 0 {
		return basePrompt
	}
	return fmt.Sprintf("%s\n\n## External Context & Injected Knowledge\n\nGround your analysis in the broader system reality. Source of truth:\n\n---\n\n%s",
		basePrompt, strings.Join(valid, "\n\n---\n\n"))
}

// CurrentDateString returns formatted current date for prompt context.
func CurrentDateString() string {
	return time.Now().Format("02/01/2006")
}

// ToJSONString converts data into formatted JSON string.
func ToJSONString(data any) string {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
