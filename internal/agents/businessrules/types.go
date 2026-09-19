// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

// TaskMetadata contains parsed ticket/issue metadata from Jira, Linear, GitHub, etc.
type TaskMetadata struct {
	ID                 string   `json:"id,omitempty"`
	Title              string   `json:"title,omitempty"`
	Description        string   `json:"description,omitempty"`
	Links              []string `json:"links,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
}

// BusinessRulesContext bundles inputs for the gap analysis agent.
type BusinessRulesContext struct {
	TaskContext           string        `json:"task_context"`
	TaskContextNormalized *TaskMetadata `json:"task_context_normalized,omitempty"`
	TaskQuality           string        `json:"task_quality"` // "strong" | "adequate" | "weak" | "missing"
	PRDiff                string        `json:"pr_diff"`
	PRBody                string        `json:"pr_body,omitempty"`
	UserLanguage          string        `json:"user_language,omitempty"`
	OrganizationID        string        `json:"organization_id,omitempty"`
	TeamID                string        `json:"team_id,omitempty"`
	RepositoryID          string        `json:"repository_id,omitempty"`
	ThreadID              string        `json:"thread_id,omitempty"`
}

// ValidationResult represents the output of a business rules validation pass.
type ValidationResult struct {
	NeedsMoreInfo         bool     `json:"needs_more_info"`
	IsCompliant           bool     `json:"is_compliant"`
	Summary               string   `json:"summary"`
	Mode                  string   `json:"mode,omitempty"`   // "full_analysis" | "limitation_response"
	Reason                string   `json:"reason,omitempty"` // "analysis_ready" | "task_context_missing" | "task_context_weak" | "pr_diff_missing" | "analyzer_failure" | "parser_fallback"
	Confidence            string   `json:"confidence,omitempty"`
	MissingInfo           string   `json:"missing_info,omitempty"`
	ViolatedRules         []string `json:"violated_rules,omitempty"`
	MissingRequirements   []string `json:"missing_requirements,omitempty"`
	Suggestions           []string `json:"suggestions,omitempty"`
	QualityClassification string   `json:"quality_classification,omitempty"`
}

// Verdict models the decision of the verifier pass.
type Verdict struct {
	Keep       bool   `json:"keep"`
	Rationale  string `json:"rationale"`
	Confidence string `json:"confidence,omitempty"`
}
