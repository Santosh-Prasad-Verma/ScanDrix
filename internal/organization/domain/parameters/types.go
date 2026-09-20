// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package paramdomain

// ParameterKey represents workspace and team-level parameter keys.
type ParameterKey string

const (
	KeyCodeReviewConfig    ParameterKey = "code_review_config"
	KeyPlatformConfigs     ParameterKey = "platform_configs"
	KeyLanguageConfig      ParameterKey = "language_config"
	KeyIssueCreationConfig ParameterKey = "issue_creation_config"
	KeyCentralizedConfig   ParameterKey = "centralized_config"
	KeyDrixyRulesConfig    ParameterKey = "drixy_rules_config"
	KeyNotificationConfigs ParameterKey = "notification_configs"

	// Deprecated / Legacy keys
	KeyTeamArtifactsConfig         ParameterKey = "team_artifacts_config"
	KeyOrganizationArtifactsConfig ParameterKey = "organization_artifacts_config"
	KeyCommunicationStyle          ParameterKey = "communication_style"
	KeyCheckinConfig               ParameterKey = "checkin_config"
	KeyBoardPriorityType           ParameterKey = "board_priority_type"
	KeyDeploymentType              ParameterKey = "deployment_type"
)

// DrixyLearningStatus defines autonomous learning modes for Drixy.
type DrixyLearningStatus string

const (
	DrixyLearningStatusEnabled          DrixyLearningStatus = "enabled"
	DrixyLearningStatusDisabled         DrixyLearningStatus = "disabled"
	DrixyLearningStatusGeneratingRules  DrixyLearningStatus = "generating_rules"
	DrixyLearningStatusGeneratingConfig DrixyLearningStatus = "generating_config"
)

// PlatformConfigValue defines onboarding and integration health flags.
type PlatformConfigValue struct {
	FinishOnboard                     bool                `json:"finishOnboard"`
	FinishProjectManagementConnection bool                `json:"finishProjectManagementConnection"`
	DrixyLearningStatus               DrixyLearningStatus `json:"drixyLearningStatus"`
	DrixyLearningStuckRetries         *int                `json:"drixyLearningStuckRetries,omitempty"`
}

// CodeReviewConfigValue defines automated pull request review behavior.
type CodeReviewConfigValue struct {
	MaxFiles                int      `json:"maxFiles"`
	IgnoredPaths            []string `json:"ignoredPaths"`
	IncludedPaths           []string `json:"includedPaths"`
	AutomatedReviewLabels   []string `json:"automatedReviewLabels"`
	SeverityThreshold       string   `json:"severityThreshold"`
	EnableInlineSuggestions bool     `json:"enableInlineSuggestions"`
	DrixyRulesEnabled       bool     `json:"drixyRulesEnabled"`
}

// DayOfWeek represents standard days for scheduled checkins.
type DayOfWeek string

const (
	DaySunday    DayOfWeek = "sun"
	DayMonday    DayOfWeek = "mon"
	DayTuesday   DayOfWeek = "tue"
	DayWednesday DayOfWeek = "wed"
	DayThursday  DayOfWeek = "thu"
	DayFriday    DayOfWeek = "fri"
	DaySaturday  DayOfWeek = "sat"
)

// CheckinFrequency maps days of week to active status.
type CheckinFrequency map[DayOfWeek]bool

// SessionFrequency defines recurrence intervals for checkin sessions.
type SessionFrequency string

const (
	SessionFrequencyDaily  SessionFrequency = "daily"
	SessionFrequencyWeekly SessionFrequency = "weekly"
)

// SectionType represents individual modules within a team checkin.
type SectionType string

const (
	SectionReleaseNotes       SectionType = "releaseNotes"
	SectionPullRequestsOpened SectionType = "pullRequestsOpened"
	SectionLateWorkItems      SectionType = "lateWorkItems"
	SectionTeamArtifacts      SectionType = "teamArtifacts"
	SectionTeamDoraMetrics    SectionType = "teamDoraMetrics"
	SectionTeamFlowMetrics    SectionType = "teamFlowMetrics"
)

// SectionConfigAdditional holds supplemental options such as sub-cadence.
type SectionConfigAdditional struct {
	Frequency *SessionFrequency `json:"frequency,omitempty"`
}

// SectionConfigItem defines an individual checkin section status and display order.
type SectionConfigItem struct {
	ID               SectionType              `json:"id"`
	Active           bool                     `json:"active"`
	Order            int                      `json:"order"`
	AdditionalConfig *SectionConfigAdditional `json:"additionalConfig,omitempty"`
}

// SectionConfig maps SectionType keys to section configurations.
type SectionConfig map[SectionType]SectionConfigItem

// CheckinConfigValue represents automated team checkin configuration.
type CheckinConfigValue struct {
	CheckinID   string           `json:"checkinId"`
	CheckinName string           `json:"checkinName"`
	Frequency   CheckinFrequency `json:"frequency"`
	Sections    SectionConfig    `json:"sections"`
	CheckinTime string           `json:"checkinTime"`
}

// CentralizedConfigRepository identifies the target repo for centralized config.
type CentralizedConfigRepository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CentralizedConfigActivePullRequest tracks an in-flight synchronization PR.
type CentralizedConfigActivePullRequest struct {
	PRURL        string                      `json:"prUrl"`
	PRNumber     *int                        `json:"prNumber,omitempty"`
	SourceBranch string                      `json:"sourceBranch"`
	TargetBranch *string                     `json:"targetBranch,omitempty"`
	Repository   CentralizedConfigRepository `json:"repository"`
	CreatedAt    string                      `json:"createdAt"`
	UpdatedAt    string                      `json:"updatedAt"`
}

// CentralizedConfigParameter defines organization-wide single-source-of-truth configuration.
type CentralizedConfigParameter struct {
	Enabled                bool                                `json:"enabled"`
	Repository             *CentralizedConfigRepository        `json:"repository"`
	ActivePullRequest      *CentralizedConfigActivePullRequest `json:"activePullRequest,omitempty"`
	ManagedRepositoryIDs   []string                            `json:"managedRepositoryIds,omitempty"`
	ManagedDirectoryScopes map[string][]string                 `json:"managedDirectoryScopes,omitempty"`
	ManagedGlobalConfig    *bool                               `json:"managedGlobalConfig,omitempty"`
}
