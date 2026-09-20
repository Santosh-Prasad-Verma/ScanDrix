package types

// ReviewRuleOverride defines custom rule toggles or overrides per repository.
type ReviewRuleOverride struct {
	RuleID   string   `json:"rule_id" yaml:"rule_id"`
	Enabled  *bool    `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Severity Severity `json:"severity,omitempty" yaml:"severity,omitempty"`
	Message  string   `json:"message,omitempty" yaml:"message,omitempty"`
}

// RepoReviewConfig defines review options inside .scandrix.yaml.
type RepoReviewConfig struct {
	AutoReviewPRs       bool                 `json:"auto_review_prs" yaml:"auto_review_prs"`
	DraftPRs            bool                 `json:"draft_prs" yaml:"draft_prs"`
	MinSeverity         Severity             `json:"min_severity" yaml:"min_severity"`
	BlockingSeverities  []Severity           `json:"blocking_severities" yaml:"blocking_severities"`
	IgnorePaths         []string             `json:"ignore_paths" yaml:"ignore_paths"`
	IncludePaths        []string             `json:"include_paths" yaml:"include_paths"`
	RuleOverrides       []ReviewRuleOverride `json:"rule_overrides" yaml:"rule_overrides"`
	MaxFilesPerReview   int                  `json:"max_files_per_review" yaml:"max_files_per_review"`
	MaxDiffBytes        int64                `json:"max_diff_bytes" yaml:"max_diff_bytes"`
	EnableASTAnalysis   bool                 `json:"enable_ast_analysis" yaml:"enable_ast_analysis"`
	EnableSuggestions   bool                 `json:"enable_suggestions" yaml:"enable_suggestions"`
	AutoFixThreshold    Severity             `json:"auto_fix_threshold" yaml:"auto_fix_threshold"`
}

// RepoTraceConfig defines trace and session capture behavior in .scandrix.yaml.
type RepoTraceConfig struct {
	Enabled             bool     `json:"enabled" yaml:"enabled"`
	AutoDistillOnPush   bool     `json:"auto_distill_on_push" yaml:"auto_distill_on_push"`
	RedactSecrets       bool     `json:"redact_secrets" yaml:"redact_secrets"`
	RedactPatterns      []string `json:"redact_patterns" yaml:"redact_patterns"`
	LocalRetentionDays  int      `json:"local_retention_days" yaml:"local_retention_days"`
	LocalServerPort     int      `json:"local_server_port" yaml:"local_server_port"`
}

// RepoConfig holds the repository-level configuration schema.
type RepoConfig struct {
	Version      string           `json:"version" yaml:"version"`
	WorkspaceID  string           `json:"workspace_id,omitempty" yaml:"workspace_id,omitempty"`
	RepositoryID string           `json:"repository_id,omitempty" yaml:"repository_id,omitempty"`
	Review       RepoReviewConfig `json:"review" yaml:"review"`
	Trace        RepoTraceConfig  `json:"trace" yaml:"trace"`
	Extensions   map[string]any   `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// GlobalConfig holds user-wide preferences in ~/.scandrix/config.json.
type GlobalConfig struct {
	APIBaseURL          string            `json:"api_base_url"`
	TelemetryEnabled    bool              `json:"telemetry_enabled"`
	DefaultFormat       string            `json:"default_format"` // terminal, json, markdown, sarif
	AutoCheckUpdates    bool              `json:"auto_check_updates"`
	LastUpdateCheck     int64             `json:"last_update_check"`
	PreferredEditor     string            `json:"preferred_editor"`
	DefaultWorkspaceID  string            `json:"default_workspace_id,omitempty"`
	EnvironmentDefaults map[string]string `json:"environment_defaults,omitempty"`
}

// DefaultRepoConfig returns a production-ready baseline configuration.
func DefaultRepoConfig() RepoConfig {
	return RepoConfig{
		Version: "v1",
		Review: RepoReviewConfig{
			AutoReviewPRs:      true,
			DraftPRs:           false,
			MinSeverity:        SeverityWarning,
			BlockingSeverities: []Severity{SeverityCritical, SeverityError},
			IgnorePaths: []string{
				"vendor/**",
				"node_modules/**",
				"dist/**",
				"build/**",
				"**/*.min.js",
				"**/*.lock",
				"go.sum",
			},
			MaxFilesPerReview: 100,
			MaxDiffBytes:      5 * 1024 * 1024, // 5MB
			EnableASTAnalysis: true,
			EnableSuggestions: true,
			AutoFixThreshold:  SeverityWarning,
		},
		Trace: RepoTraceConfig{
			Enabled:            true,
			AutoDistillOnPush:  true,
			RedactSecrets:      true,
			LocalRetentionDays: 30,
			LocalServerPort:    57890,
		},
	}
}

// DefaultGlobalConfig returns defaults for user CLI configuration.
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		APIBaseURL:       "https://api.scandrix.dev",
		TelemetryEnabled: true,
		DefaultFormat:    "terminal",
		AutoCheckUpdates: true,
	}
}
