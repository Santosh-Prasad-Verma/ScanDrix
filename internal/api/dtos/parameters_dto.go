package dtos

// ReviewParametersDTO holds repository or workspace level review behavior settings.
type ReviewParametersDTO struct {
	DefaultAIModel           string   `json:"default_ai_model"`
	MaxDiffLines             int      `json:"max_diff_lines"`
	DryRunMode               bool     `json:"dry_run_mode"`
	AutoApproveCleanPRs      bool     `json:"auto_approve_clean_prs"`
	EnforceConventionalTitle bool     `json:"enforce_conventional_title"`
	IgnorePatterns           []string `json:"ignore_patterns"`
	CustomSystemPrompt       string   `json:"custom_system_prompt"`
}

// OrgParametersDTO holds organization-wide enforcement rules.
type OrgParametersDTO struct {
	BlockPRMergeOnCritical     bool   `json:"block_pr_merge_on_critical"`
	RequireReviewDismissalRole string `json:"require_review_dismissal_role"`
	DefaultBranchOnly          bool   `json:"default_branch_only"`
	NotificationSlackChannel   string `json:"notification_slack_channel"`
}
