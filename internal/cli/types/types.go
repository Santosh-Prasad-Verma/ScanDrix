package types

import (
	"time"
)

// Severity represents finding severity level.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityError    Severity = "error"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// PlatformType represents the git hosting provider.
type PlatformType string

const (
	PlatformGitHub    PlatformType = "github"
	PlatformGitLab    PlatformType = "gitlab"
	PlatformBitbucket PlatformType = "bitbucket"
	PlatformAzure     PlatformType = "azure"
	PlatformUnknown   PlatformType = "unknown"
)

// GitInfo holds detected repository metadata.
type GitInfo struct {
	RootPath      string       `json:"root_path"`
	Branch        string       `json:"branch"`
	HeadSHA       string       `json:"head_sha"`
	RemoteURL     string       `json:"remote_url"`
	Platform      PlatformType `json:"platform"`
	Owner         string       `json:"owner"`
	Repo          string       `json:"repo"`
	IsClean       bool         `json:"is_clean"`
	HooksDir      string       `json:"hooks_dir"`
	CommonDir     string       `json:"common_dir"`
	IsWorktree    bool         `json:"is_worktree"`
	TrackingAhead int          `json:"tracking_ahead"`
	TrackingBehind int         `json:"tracking_behind"`
}

// FileDiff holds single file diff information.
type FileDiff struct {
	Path         string `json:"path"`
	OldPath      string `json:"old_path,omitempty"`
	Status       string `json:"status"` // added, modified, deleted, renamed
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Patch        string `json:"patch"`
	IsBinary     bool   `json:"is_binary"`
	OldMode      string `json:"old_mode,omitempty"`
	NewMode      string `json:"new_mode,omitempty"`
}

// FileContent holds file contents along with diff and status.
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Status  string `json:"status,omitempty"` // added, modified, deleted, renamed
	Diff    string `json:"diff,omitempty"`
	SHA     string `json:"sha,omitempty"`
	Lines   int    `json:"lines,omitempty"`
}

// CodeFix represents an automated fix recommendation.
type CodeFix struct {
	Explanation string `json:"explanation"`
	OldCode     string `json:"old_code"`
	NewCode     string `json:"new_code"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	StartCol    int    `json:"start_col,omitempty"`
	EndCol      int    `json:"end_col,omitempty"`
}

// ReviewIssue represents a discrete code finding produced by ScanDrix or Drixy.
type ReviewIssue struct {
	ID             string    `json:"id"`
	File           string    `json:"file"`
	Line           int       `json:"line"`
	EndLine        int       `json:"end_line,omitempty"`
	Column         int       `json:"column,omitempty"`
	EndColumn      int       `json:"end_column,omitempty"`
	Severity       Severity  `json:"severity"`
	Category       string    `json:"category"` // security, reliability, style, performance, architecture
	Message        string    `json:"message"`
	Recommendation string    `json:"recommendation,omitempty"`
	Suggestion     string    `json:"suggestion,omitempty"`
	RuleID         string    `json:"rule_id,omitempty"`
	Fixable        bool      `json:"fixable"`
	Fix            *CodeFix  `json:"fix,omitempty"`
	HunkContext    string    `json:"hunk_context,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
}

// ReviewStats holds summary issue count metrics.
type ReviewStats struct {
	CriticalCount int `json:"critical_count"`
	ErrorCount    int `json:"error_count"`
	WarningCount  int `json:"warning_count"`
	InfoCount     int `json:"info_count"`
	FixableCount  int `json:"fixable_count"`
	TotalIssues   int `json:"total_issues"`
}

// ReviewResult aggregates complete review outcome.
type ReviewResult struct {
	ReviewID      string        `json:"review_id"`
	Status        string        `json:"status"` // "passed", "failed", "warning"
	Summary       string        `json:"summary"`
	FilesAnalyzed int           `json:"files_analyzed"`
	Issues        []ReviewIssue `json:"issues"`
	Stats         ReviewStats   `json:"stats"`
	Duration      time.Duration `json:"duration"`
	DurationMs    int64         `json:"duration_ms"`
	IsBlocking    bool          `json:"is_blocking"`
	ExitCode      int           `json:"exit_code"`
	FixesApplied  int           `json:"fixes_applied,omitempty"`
	GitContext    *GitInfo      `json:"git_context,omitempty"`
}

// ReviewOptions holds all user-specified flags for review command.
type ReviewOptions struct {
	Staged         bool          `json:"staged"`
	Branch         string        `json:"branch,omitempty"`
	Commit         string        `json:"commit,omitempty"`
	All            bool          `json:"all"`
	Files          []string      `json:"files,omitempty"`
	Format         string        `json:"format"` // terminal, json, markdown, sarif
	Output         string        `json:"output,omitempty"`
	Verbose        bool          `json:"verbose"`
	Quiet          bool          `json:"quiet"`
	Agent          bool          `json:"agent"` // deterministic output for AI agents
	ApplyFixes     bool          `json:"apply_fixes"`
	Interactive    bool          `json:"interactive"`
	RuleGroup      string        `json:"rule_group,omitempty"`
	RuleIDs        []string      `json:"rule_ids,omitempty"`
	MinSeverity    Severity      `json:"min_severity,omitempty"`
	Timeout        time.Duration `json:"timeout"`
	Offline        bool          `json:"offline"`
	DryRun         bool          `json:"dry_run"`
	NoHunk         bool          `json:"no_hunk"`
	WorkspaceID    string        `json:"workspace_id,omitempty"`
	RepositoryID   string        `json:"repository_id,omitempty"`
	TeamKey        string        `json:"team_key,omitempty"`
}
