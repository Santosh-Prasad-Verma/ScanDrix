package dtos

import (
	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// CreateRuleRequest defines fields for authoring custom review rules.
type CreateRuleRequest struct {
	Name        string                 `json:"name"`
	PathPattern string                 `json:"path_pattern"`
	RegexRule   string                 `json:"regex_rule"`
	Severity    models.FindingSeverity `json:"severity"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Remediation string                 `json:"remediation"`
	Enabled     bool                   `json:"enabled"`
}

// UpdateRuleRequest defines fields for editing an existing rule.
type UpdateRuleRequest struct {
	Name        *string                 `json:"name,omitempty"`
	PathPattern *string                 `json:"path_pattern,omitempty"`
	RegexRule   *string                 `json:"regex_rule,omitempty"`
	Severity    *models.FindingSeverity `json:"severity,omitempty"`
	Description *string                 `json:"description,omitempty"`
	Remediation *string                 `json:"remediation,omitempty"`
	Enabled     *bool                   `json:"enabled,omitempty"`
}

// TestRuleRequest allows testing a rule regex against a code snippet in real-time.
type TestRuleRequest struct {
	RegexRule   string `json:"regex_rule"`
	CodeSnippet string `json:"code_snippet"`
	FilePath    string `json:"file_path"`
}

// RuleTestFile is one file to match, identified so a match can be attributed.
type RuleTestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// TestRuleBatchRequest matches one pattern against many files in a single call.
//
// The dashboard used to POST /rules/test once per file, which for a dry run over
// a handful of pull requests meant a few hundred sequential round trips.
type TestRuleBatchRequest struct {
	RegexRule string         `json:"regex_rule"`
	Files     []RuleTestFile `json:"files"`
}

// RuleTestFileResult reports the matches within one file.
type RuleTestFileResult struct {
	Path          string   `json:"path"`
	Matched       bool     `json:"matched"`
	MatchLines    []int    `json:"match_lines"`
	MatchSnippets []string `json:"match_snippets"`
}

// TestRuleBatchResponse reports per-file matches plus the real denominators a
// precision verdict needs: how many files were scanned and how many fired.
type TestRuleBatchResponse struct {
	Files []RuleTestFileResult `json:"files"`
	// FilesScanned is the denominator. Without it a finding count says nothing
	// about whether a rule is precise.
	FilesScanned int `json:"files_scanned"`
	FilesMatched int `json:"files_matched"`
	// MatchRate is FilesMatched / FilesScanned, the fraction of scanned files
	// the pattern fired on. Zero files scanned yields 0, not 1.
	MatchRate   float64 `json:"match_rate"`
	TotalLines  int     `json:"total_lines"`
	Truncated   bool    `json:"truncated"`
	CompileMs   int64   `json:"compile_ms"`
	MatchTimeMs int64   `json:"match_time_ms"`
}

// TestRuleResponse reports test match results.
type TestRuleResponse struct {
	Matched       bool     `json:"matched"`
	MatchLines    []int    `json:"match_lines"`
	MatchSnippets []string `json:"match_snippets"`
	// ExecutionTimeMs is the time spent compiling the pattern and matching
	// every line, measured here. The dashboard used to time its own fetch and
	// label the round trip as the rule's runtime.
	ExecutionTimeMs int64 `json:"execution_time_ms"`
}

// RuleResponse returns detailed rule definition.
type RuleResponse struct {
	ID          uuid.UUID              `json:"id"`
	WorkspaceID uuid.UUID              `json:"workspace_id"`
	Name        string                 `json:"name"`
	PathPattern string                 `json:"path_pattern"`
	RegexRule   string                 `json:"regex_rule"`
	Severity    models.FindingSeverity `json:"severity"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Remediation string                 `json:"remediation"`
	Enabled     bool                   `json:"enabled"`
}

// GenerateRuleRequest requests AI synthesis of a custom Drixy rule.
type GenerateRuleRequest struct {
	Prompt      string `json:"prompt"`
	CodeContext string `json:"code_context,omitempty"`
	Language    string `json:"language,omitempty"`
}

// GenerateRuleResponse returns the generated Drixy rule.
type GenerateRuleResponse struct {
	Name        string                 `json:"name"`
	RegexRule   string                 `json:"regex_rule"`
	PathPattern string                 `json:"path_pattern"`
	Severity    models.FindingSeverity `json:"severity"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Remediation string                 `json:"remediation"`
	Explanation string                 `json:"explanation,omitempty"`
}
