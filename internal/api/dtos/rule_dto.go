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

// TestRuleResponse reports test match results.
type TestRuleResponse struct {
	Matched       bool     `json:"matched"`
	MatchLines    []int    `json:"match_lines"`
	MatchSnippets []string `json:"match_snippets"`
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
