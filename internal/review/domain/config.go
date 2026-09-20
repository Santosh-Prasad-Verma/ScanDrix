package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ModelStrictness specifies the evaluation strictness of the review model.
type ModelStrictness string

const (
	StrictnessStrict     ModelStrictness = "STRICT"
	StrictnessBalanced   ModelStrictness = "BALANCED"
	StrictnessLenient    ModelStrictness = "LENIENT"
	StrictnessPermissive ModelStrictness = "PERMISSIVE"
)

// ReviewSeverity defines the impact level of an identified finding.
type ReviewSeverity string

const (
	SeverityCritical ReviewSeverity = "CRITICAL"
	SeverityHigh     ReviewSeverity = "HIGH"
	SeverityMajor    ReviewSeverity = "MAJOR"
	SeverityMedium   ReviewSeverity = "MEDIUM"
	SeverityMinor    ReviewSeverity = "MINOR"
	SeverityLow      ReviewSeverity = "LOW"
	SeverityInfo     ReviewSeverity = "INFO"
)

// ReviewCategory classifies the nature of a code review recommendation.
type ReviewCategory string

const (
	CategorySecurity      ReviewCategory = "SECURITY"
	CategoryPerformance   ReviewCategory = "PERFORMANCE"
	CategoryBug           ReviewCategory = "BUG"
	CategoryArchitecture  ReviewCategory = "ARCHITECTURE"
	CategoryRules         ReviewCategory = "RULES"
	CategoryBusinessLogic ReviewCategory = "BUSINESS_LOGIC"
	CategoryStyle         ReviewCategory = "STYLE"
)

// LanguageConfig holds per-language tuning flags.
type LanguageConfig struct {
	Enabled        bool     `json:"enabled"`
	Linters        []string `json:"linters,omitempty"`
	Strictness     string   `json:"strictness,omitempty"`
	CustomRules    []string `json:"customRules,omitempty"`
	ExcludeTest    bool     `json:"excludeTest,omitempty"`
	FileExtensions []string `json:"fileExtensions,omitempty"`
}

// FileSizeLimits bounds the review scope per file to protect LLM context windows.
type FileSizeLimits struct {
	MaxLines int `json:"maxLines"`
	MaxBytes int `json:"maxBytes"`
}

// ReviewCadenceType defines the execution trigger strategy for pull requests.
type ReviewCadenceType string

const (
	CadenceAutomatic ReviewCadenceType = "AUTOMATIC"
	CadenceManual    ReviewCadenceType = "MANUAL"
	CadenceAutoPause ReviewCadenceType = "AUTO_PAUSE"
)

// ReviewCadenceState records the lifecycle pause/run state of a pull request.
type ReviewCadenceState string

const (
	CadenceStateAutomatic ReviewCadenceState = "AUTOMATIC"
	CadenceStatePaused    ReviewCadenceState = "PAUSED"
	CadenceStateCommand   ReviewCadenceState = "COMMAND"
)

// ReviewCadenceConfig parametrizes cadence behavior and burst push suppression.
type ReviewCadenceConfig struct {
	Type              ReviewCadenceType `json:"type"`
	PushesToTrigger   int               `json:"pushesToTrigger"`
	TimeWindowMinutes int               `json:"timeWindow"`
}

// BehaviourForNewCommits dictates how PR summaries update when subsequent commits arrive.
type BehaviourForNewCommits string

const (
	CommitBehaviourNone    BehaviourForNewCommits = "NONE"
	CommitBehaviourReplace BehaviourForNewCommits = "REPLACE"
	CommitBehaviourAppend  BehaviourForNewCommits = "APPEND"
)

// SummaryConfig controls whether top-level PR summarization is enabled and its incremental behavior.
type SummaryConfig struct {
	GeneratePRSummary       bool                   `json:"generatePRSummary"`
	BehaviourForNewCommits  BehaviourForNewCommits `json:"behaviourForNewCommits"`
}

// ResolvedModelSlotInfo captures the AI provider model routing resolution.
type ResolvedModelSlotInfo struct {
	ModelID               string `json:"modelId"`
	ModelName             string `json:"modelName"`
	Provider              string `json:"provider"`
	MaxConcurrentRequests int    `json:"maxConcurrentRequests,omitempty"`
}

// ReviewOptions enables or disables specific automated analysis engines.
type ReviewOptions struct {
	BusinessLogic bool `json:"business_logic"`
	Security      bool `json:"security"`
	Performance   bool `json:"performance"`
	Bug           bool `json:"bug"`
	Architecture  bool `json:"architecture"`
}

// CodeReviewConfig embodies the resolved configuration applied during review execution.
type CodeReviewConfig struct {
	Enabled                      bool                      `json:"enabled"`
	AutomatedReviewActive        bool                      `json:"automatedReviewActive"`
	AutoApprove                  bool                      `json:"autoApprove"`
	PullRequestApprovalActive    bool                      `json:"pullRequestApprovalActive"`
	IsRequestChangesActive       bool                      `json:"isRequestChangesActive"`
	ApprovalThreshold            float64                   `json:"approvalThreshold"`
	Strictness                   ModelStrictness           `json:"strictness"`
	IgnorePaths                  []string                  `json:"ignorePaths"`
	IgnorePatterns               []string                  `json:"ignorePatterns,omitempty"`
	IgnoredTitleKeywords         []string                  `json:"ignoredTitleKeywords,omitempty"`
	BaseBranches                 []string                  `json:"baseBranches,omitempty"`
	BaseBranchDefault            string                    `json:"baseBranchDefault,omitempty"`
	RunOnDraft                   bool                      `json:"runOnDraft"`
	ShowStatusFeedback           bool                      `json:"showStatusFeedback"`
	ReviewCadence                ReviewCadenceConfig       `json:"reviewCadence"`
	Summary                      SummaryConfig             `json:"summary"`
	FocusPaths                   []string                  `json:"focusPaths,omitempty"`
	Languages                    map[string]LanguageConfig `json:"languages,omitempty"`
	MaxSuggestions               int                       `json:"maxSuggestions"`
	MaxFilesPerReview            int                       `json:"maxFilesPerReview"`
	Rules                        []string                  `json:"rules,omitempty"`
	ModelOverrides               map[string]string         `json:"modelOverrides,omitempty"`
	ByokModel                    string                    `json:"byokModel,omitempty"`
	ByokModelID                  string                    `json:"byokModelId,omitempty"`
	ResolvedModelSlot            *ResolvedModelSlotInfo    `json:"resolvedModelSlot,omitempty"`
	FileSizeLimits               FileSizeLimits            `json:"fileSizeLimits"`
	NotifyOnSlack                bool                      `json:"notifyOnSlack"`
	NotifyOnTeams                bool                      `json:"notifyOnTeams"`
	ReviewHeavyMode              bool                      `json:"reviewHeavyMode"`
	RequireTicketContext         bool                      `json:"requireTicketContext"`
	SyntaxCheckInSandbox         bool                      `json:"syntaxCheckInSandbox"`
	DisputeHandlingPolicy        string                    `json:"disputeHandlingPolicy"` // "auto_suppress", "flag_disputed"
	LanguageResultPrompt         string                    `json:"languageResultPrompt,omitempty"`
	ReviewOptions                ReviewOptions             `json:"reviewOptions"`
	EnableCommittableSuggestions bool                      `json:"enableCommittableSuggestions"`
	ReviewMode                   string                    `json:"reviewMode,omitempty"`
	Sensitivity                  string                    `json:"sensitivity,omitempty"`
}

// DefaultCodeReviewConfig returns production baseline defaults for code reviews.
func DefaultCodeReviewConfig() CodeReviewConfig {
	return CodeReviewConfig{
		Enabled:                   true,
		AutomatedReviewActive:     true,
		AutoApprove:               false,
		PullRequestApprovalActive: true,
		IsRequestChangesActive:    true,
		ReviewMode:                "normal",
		Sensitivity:               "STANDARD",
		ApprovalThreshold:         0.95,
		Strictness:                StrictnessBalanced,
		IgnorePaths: []string{
			"vendor/**", "node_modules/**", "dist/**", "build/**",
			"*.min.js", "*.min.css", "*.lock", "package-lock.json",
			"pnpm-lock.yaml", "yarn.lock", "go.sum", "*.pb.go",
		},
		IgnoredTitleKeywords: []string{"[skip-ci]", "[skip-review]", "[drixy-skip]", "[scandrix-skip]", "wip:"},
		RunOnDraft:           false,
		ShowStatusFeedback:   true,
		ReviewCadence: ReviewCadenceConfig{
			Type:              CadenceAutomatic,
			PushesToTrigger:   3,
			TimeWindowMinutes: 15,
		},
		Summary: SummaryConfig{
			GeneratePRSummary:      true,
			BehaviourForNewCommits: CommitBehaviourReplace,
		},
		MaxSuggestions:       25,
		MaxFilesPerReview:    100,
		FileSizeLimits: FileSizeLimits{
			MaxLines: 1500,
			MaxBytes: 250 * 1024,
		},
		ReviewHeavyMode:      false,
		RequireTicketContext: false,
		SyntaxCheckInSandbox: true,
		LanguageResultPrompt: "en-US",
		ReviewOptions: ReviewOptions{
			BusinessLogic: true,
			Security:      true,
			Performance:   true,
			Bug:           true,
			Architecture:  true,
		},
		EnableCommittableSuggestions: true,
	}
}

// CodeReviewParameter represents stored entity configuration (org, team, repo).
type CodeReviewParameter struct {
	ID             uuid.UUID        `json:"id"`
	OrganizationID string           `json:"organizationId"`
	TeamID         string           `json:"teamId"`
	RepositoryID   string           `json:"repositoryId,omitempty"` // empty for org/team level
	Config         CodeReviewConfig `json:"config"`
	Repositories   []string         `json:"repositories,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// AutomationLabel represents an SCM label that triggers or suppresses review.
type AutomationLabel struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Action      string `json:"action"` // "trigger_review", "skip_review", "deep_review"
	IsActive    bool   `json:"isActive"`
}

// DefaultAutomationLabels returns standard labels supported by ScanDrix.
func DefaultAutomationLabels() []AutomationLabel {
	return []AutomationLabel{
		{Name: "scandrix:review", Description: "Trigger immediate ScanDrix review", Action: "trigger_review", IsActive: true},
		{Name: "scandrix:skip", Description: "Skip automated ScanDrix review", Action: "skip_review", IsActive: true},
		{Name: "scandrix:deep", Description: "Execute deep multi-turn analysis", Action: "deep_review", IsActive: true},
		{Name: "scandrix:security", Description: "Run enhanced security audit sweep", Action: "trigger_review", IsActive: true},
	}
}

// IsPathIgnored evaluates a file path against configured glob ignore patterns.
func (c *CodeReviewConfig) IsPathIgnored(path string) bool {
	normalized := strings.ToLower(strings.TrimSpace(path))
	for _, pattern := range c.IgnorePaths {
		pat := strings.ToLower(strings.TrimSpace(pattern))
		if pat == "" {
			continue
		}
		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if strings.HasPrefix(normalized, prefix+"/") || normalized == prefix {
				return true
			}
		} else if strings.HasPrefix(pat, "*.") {
			ext := strings.TrimPrefix(pat, "*")
			if strings.HasSuffix(normalized, ext) {
				return true
			}
		} else if normalized == pat {
			return true
		}
	}
	return false
}

// MatchesPathPatterns evaluates a file path against a provided list of glob patterns.
func (c *CodeReviewConfig) MatchesPathPatterns(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return c.IsPathIgnored(path)
	}
	normalized := strings.ToLower(strings.TrimSpace(path))
	for _, pattern := range patterns {
		pat := strings.ToLower(strings.TrimSpace(pattern))
		if pat == "" {
			continue
		}
		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if strings.HasPrefix(normalized, prefix+"/") || normalized == prefix {
				return true
			}
		} else if strings.HasPrefix(pat, "*.") {
			ext := strings.TrimPrefix(pat, "*")
			if strings.HasSuffix(normalized, ext) {
				return true
			}
		} else if normalized == pat {
			return true
		}
	}
	return false
}
