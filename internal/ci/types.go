// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"time"

	"github.com/google/uuid"
)

// CIPlatform identifies the CI/CD execution environment.
type CIPlatform string

const (
	PlatformGitHubActions       CIPlatform = "github-actions"
	PlatformGitLabCI            CIPlatform = "gitlab-ci"
	PlatformAzurePipelines      CIPlatform = "azure-pipelines"
	PlatformBitbucketPipelines  CIPlatform = "bitbucket-pipelines"
	PlatformForgejoCI           CIPlatform = "forgejo-ci"
	PlatformGeneric             CIPlatform = "generic"
)

// SeverityThreshold represents the gate threshold that will fail a build.
type SeverityThreshold string

const (
	FailNever    SeverityThreshold = "never"
	FailInfo     SeverityThreshold = "info"
	FailWarning  SeverityThreshold = "warning"
	FailError    SeverityThreshold = "error"
	FailCritical SeverityThreshold = "critical"
)

// CIExitCode represents standardized process exit codes for CI environments.
type CIExitCode int

const (
	ExitSuccess            CIExitCode = 0
	ExitQualityGateFailed  CIExitCode = 1
	ExitReviewError        CIExitCode = 2
	ExitConfigurationError CIExitCode = 3
)

// CIEnvironment contains metadata auto-detected from CI runner environment variables.
type CIEnvironment struct {
	Platform       CIPlatform `json:"platform"`
	RepoOwner      string     `json:"repo_owner"`
	RepoName       string     `json:"repo_name"`
	CommitSHA      string     `json:"commit_sha"`
	BaseRef        string     `json:"base_ref"`
	HeadRef        string     `json:"head_ref"`
	PullRequestID  int        `json:"pull_request_id"`
	JobID          string     `json:"job_id"`
	RunID          string     `json:"run_id"`
	WorkspacePath  string     `json:"workspace_path"`
	APIToken       string     `json:"api_token,omitempty"`
	ServerURL      string     `json:"server_url,omitempty"`
	IsPR           bool       `json:"is_pr"`
	Actor          string     `json:"actor"`
	EventName      string     `json:"event_name"`
}

// CIFinding represents a single normalized defect, security vulnerability, or rule violation.
type CIFinding struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id"`
	RuleTitle   string            `json:"rule_title"`
	Category    string            `json:"category"` // "SECURITY", "PERFORMANCE", "BUG", "ARCHITECTURE", "DRIXY_RULES"
	Severity    SeverityThreshold `json:"severity"` // "critical", "error", "warning", "info"
	FilePath    string            `json:"file_path"`
	StartLine   int               `json:"start_line"`
	EndLine     int               `json:"end_line"`
	Message     string            `json:"message"`
	Description string            `json:"description,omitempty"`
	Suggestion  string            `json:"suggestion,omitempty"`
	CWETaxonomy string            `json:"cwe_taxonomy,omitempty"`
	Confidence  float64           `json:"confidence"`
}

// HeadlessReviewConfig specifies execution settings for the automated review runner.
type HeadlessReviewConfig struct {
	FailOnSeverity   SeverityThreshold `json:"fail_on_severity"`
	TargetBranch     string            `json:"target_branch"`
	DiffFilePath     string            `json:"diff_file_path,omitempty"`
	WorkspaceDir     string            `json:"workspace_dir"`
	OutputSARIF      string            `json:"output_sarif,omitempty"`
	OutputJUnit      string            `json:"output_junit,omitempty"`
	OutputGitLab     string            `json:"output_gitlab,omitempty"`
	OutputMarkdown   string            `json:"output_markdown,omitempty"`
	Token            string            `json:"token,omitempty"`
	OrganizationID   uuid.UUID         `json:"organization_id,omitempty"`
	MaxFindings      int               `json:"max_findings"`
	IncludePaths     []string          `json:"include_paths,omitempty"`
	ExcludePaths     []string          `json:"exclude_paths,omitempty"`
	SkipCommitFlag   bool              `json:"skip_commit_flag"`
}

// HeadlessReviewResult aggregates findings, metrics, and quality gate conclusions.
type HeadlessReviewResult struct {
	GatePassed      bool              `json:"gate_passed"`
	ExitCode        CIExitCode        `json:"exit_code"`
	GateReason      string            `json:"gate_reason"`
	TotalFindings   int               `json:"total_findings"`
	CriticalCount   int               `json:"critical_count"`
	ErrorCount      int               `json:"error_count"`
	WarningCount    int               `json:"warning_count"`
	InfoCount       int               `json:"info_count"`
	Findings        []CIFinding       `json:"findings"`
	ReviewedFiles   []string          `json:"reviewed_files"`
	Duration        time.Duration     `json:"duration"`
	GeneratedAt     time.Time         `json:"generated_at"`
	Environment     CIEnvironment     `json:"environment"`
	CheckRunID      string            `json:"check_run_id,omitempty"`
}
