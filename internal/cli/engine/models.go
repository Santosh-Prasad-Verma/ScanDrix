// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package engine

import (
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// OutputFormat defines the CLI report rendering target.
type OutputFormat string

const (
	FormatTable    OutputFormat = "table"
	FormatTerminal OutputFormat = "terminal"
	FormatJSON     OutputFormat = "json"
	FormatMarkdown OutputFormat = "markdown"
	FormatSARIF    OutputFormat = "sarif"
	FormatCSV      OutputFormat = "csv"
	FormatAgent    OutputFormat = "agent"
	FormatPrompt   OutputFormat = "prompt"
)

// CLIOptions encapsulates flags passed to the ScanDrix CLI binary.
type CLIOptions struct {
	Staged            bool                                            `json:"staged"`
	Branch            string                                          `json:"branch"`
	Commit            string                                          `json:"commit"`
	CommitRange       string                                          `json:"commit_range"`
	TargetDirectory   string                                          `json:"target_directory"`
	OutputFile        string                                          `json:"output_file,omitempty"`
	Offline           bool                                            `json:"offline"`
	DryRun            bool                                            `json:"dry_run"`
	Format            OutputFormat                                    `json:"format"`
	AgentMode         bool                                            `json:"agent_mode"`
	Verbose           bool                                            `json:"verbose"`
	Quiet             bool                                            `json:"quiet"`
	SeverityThreshold models.FindingSeverity                          `json:"severity_threshold"`
	APIKey            string                                          `json:"api_key,omitempty"`
	APIBaseURL        string                                          `json:"api_base_url,omitempty"`
	AccessToken       string                                          `json:"access_token,omitempty"`
	RulesOnly         bool                                            `json:"rules_only"`
	Fast              bool                                            `json:"fast"`
	Heavy             bool                                            `json:"heavy"`
	Focus             string                                          `json:"focus,omitempty"`
	Fix               bool                                            `json:"fix"`
	PromptOnly        bool                                            `json:"prompt_only"`
	ContextFile       string                                          `json:"context_file,omitempty"`
	FieldMask         string                                          `json:"field_mask,omitempty"`
	GitHubPAT         string                                          `json:"github_pat,omitempty"`
	CustomRulesFile   string                                          `json:"custom_rules_file,omitempty"`
	PromptOverride    string                                          `json:"prompt_override,omitempty"`
	OnProgress        func(relPath string, current int, lineCount int) `json:"-"`
	OnStatus          func(statusMsg string)                         `json:"-"`
}

// AgentEnvelope wraps command results into a deterministic machine-readable payload.
type AgentEnvelope struct {
	Command    string              `json:"command"`
	Status     string              `json:"status"` // "success" or "error"
	Data       any                 `json:"data,omitempty"`
	Error      *AgentEnvelopeError `json:"error,omitempty"`
	StartedAt  time.Time           `json:"started_at"`
	DurationMs int64               `json:"duration_ms"`
}

// AgentEnvelopeError details an execution failure in agent mode.
type AgentEnvelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// CLIResult aggregates findings, statistics, exit code, and remediation metadata.
type CLIResult struct {
	Status        string               `json:"status"`
	FilesReviewed int                  `json:"files_reviewed"`
	TotalLines    int                  `json:"total_lines,omitempty"`
	TotalFindings int                  `json:"total_findings"`
	CriticalCount int                  `json:"critical_count"`
	HighCount     int                  `json:"high_count"`
	MediumCount   int                  `json:"medium_count"`
	LowCount      int                  `json:"low_count"`
	Summary       string               `json:"summary,omitempty"`
	Findings      []models.CodeFinding `json:"findings"`
	ExitCode      int                  `json:"exit_code"`
	Duration      time.Duration        `json:"duration"`
	IsBlocking    bool                 `json:"is_blocking"`
	FixesApplied  int                  `json:"fixes_applied,omitempty"`
}

// SARIFLog represents the root of a SARIF v2.1.0 report for CI integrations.
type SARIFLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun details tool execution and results in SARIF.
type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

// SARIFTool defines driver metadata.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver encapsulates tool identification.
type SARIFDriver struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	InformationURI string `json:"informationUri"`
}

// SARIFResult specifies an individual finding in SARIF.
type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"` // "error", "warning", "note"
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations"`
}

// SARIFMessage holds message text.
type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFLocation maps the finding to a physical file and line.
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

// SARIFPhysicalLocation points to the file URI and region.
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

// SARIFArtifactLocation contains the URI.
type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

// SARIFRegion contains the line number.
type SARIFRegion struct {
	StartLine int `json:"startLine"`
}
