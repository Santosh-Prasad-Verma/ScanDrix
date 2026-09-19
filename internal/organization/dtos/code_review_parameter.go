// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/google/uuid"
)

// ReviewOptionsDTO controls specific categories evaluated during code review.
type ReviewOptionsDTO struct {
	Security                    *bool `json:"security,omitempty"`
	CodeStyle                   *bool `json:"code_style,omitempty"`
	Refactoring                 *bool `json:"refactoring,omitempty"`
	ErrorHandling               *bool `json:"error_handling,omitempty"`
	Maintainability             *bool `json:"maintainability,omitempty"`
	PotentialIssues             *bool `json:"potential_issues,omitempty"`
	DocumentationAndComments    *bool `json:"documentation_and_comments,omitempty"`
	PerformanceAndOptimization  *bool `json:"performance_and_optimization,omitempty"`
	DrixyRules                  *bool `json:"drixy_rules,omitempty"`
	BreakingChanges             *bool `json:"breaking_changes,omitempty"`
	Bug                         *bool `json:"bug,omitempty"`
	Performance                 *bool `json:"performance,omitempty"`
	CrossFile                   *bool `json:"cross_file,omitempty"`
	BusinessLogic               *bool `json:"business_logic,omitempty"`
}

// SummaryConfigDTO controls pull request summary generation.
type SummaryConfigDTO struct {
	GeneratePRSummary                *bool   `json:"generatePRSummary,omitempty"`
	CustomInstructions               *string `json:"customInstructions,omitempty"`
	BehaviourForExistingDescription  *string `json:"behaviourForExistingDescription,omitempty"`
	BehaviourForNewCommits           *string `json:"behaviourForNewCommits,omitempty"`
}

// SeverityLimitsDTO defines suggestion quotas per severity level.
type SeverityLimitsDTO struct {
	Low      *int `json:"low,omitempty"`
	Medium   *int `json:"medium,omitempty"`
	High     *int `json:"high,omitempty"`
	Critical *int `json:"critical,omitempty"`
}

// SuggestionControlConfigDTO governs review volume, grouping, and threshold filtering.
type SuggestionControlConfigDTO struct {
	GroupingMode             *string            `json:"groupingMode,omitempty"`
	LimitationType           *string            `json:"limitationType,omitempty"`
	MaxSuggestions           *int               `json:"maxSuggestions,omitempty"`
	SeverityLevelFilter      *string            `json:"severityLevelFilter,omitempty"`
	ApplyFiltersToDrixyRules *bool              `json:"applyFiltersToDrixyRules,omitempty"`
	SeverityLimits           *SeverityLimitsDTO `json:"severityLimits,omitempty"`
}

// ReviewCadenceDTO specifies automated review trigger frequency.
type ReviewCadenceDTO struct {
	Type            *string `json:"type,omitempty"`
	TimeWindow      *int    `json:"timeWindow,omitempty"`
	PushesToTrigger *int    `json:"pushesToTrigger,omitempty"`
}

// V2PromptOverridesSeverityFlagsDTO customizes severity instructions.
type V2PromptOverridesSeverityFlagsDTO struct {
	Critical *string `json:"critical,omitempty"`
	High     *string `json:"high,omitempty"`
	Medium   *string `json:"medium,omitempty"`
	Low      *string `json:"low,omitempty"`
}

// V2PromptOverridesSeverityDTO wraps prompt severity flag overrides.
type V2PromptOverridesSeverityDTO struct {
	Flags *V2PromptOverridesSeverityFlagsDTO `json:"flags,omitempty"`
}

// V2PromptOverridesCategoriesDescriptionsDTO defines category prompt instructions.
type V2PromptOverridesCategoriesDescriptionsDTO struct {
	Bug         *string `json:"bug,omitempty"`
	Performance *string `json:"performance,omitempty"`
	Security    *string `json:"security,omitempty"`
}

// V2PromptOverridesCategoriesDTO wraps category prompt descriptions.
type V2PromptOverridesCategoriesDTO struct {
	Descriptions *V2PromptOverridesCategoriesDescriptionsDTO `json:"descriptions,omitempty"`
}

// V2PromptOverridesGenerationDTO defines top-level generation prompt instructions.
type V2PromptOverridesGenerationDTO struct {
	Main *string `json:"main,omitempty"`
}

// V2PromptOverridesLevelDTO defines granularity overrides.
type V2PromptOverridesLevelDTO struct {
	Critical *string `json:"critical,omitempty"`
	Issue    *string `json:"issue,omitempty"`
	Warning  *string `json:"warning,omitempty"`
}

// V2PromptOverridesDTO groups all system prompt customizations.
type V2PromptOverridesDTO struct {
	Categories *V2PromptOverridesCategoriesDTO `json:"categories,omitempty"`
	Severity   *V2PromptOverridesSeverityDTO   `json:"severity,omitempty"`
	Level      *V2PromptOverridesLevelDTO      `json:"level,omitempty"`
	Generation *V2PromptOverridesGenerationDTO `json:"generation,omitempty"`
}

// CustomMessagesGlobalSettingsDTO toggles comment visibility.
type CustomMessagesGlobalSettingsDTO struct {
	HideComments *bool `json:"hideComments,omitempty"`
}

// CustomMessagesStartReviewMessageDTO configures start-of-review message.
type CustomMessagesStartReviewMessageDTO struct {
	Status  *string `json:"status,omitempty"`
	Content *string `json:"content,omitempty"`
}

// CustomMessagesEndReviewMessageDTO configures conclusion message.
type CustomMessagesEndReviewMessageDTO struct {
	Status  *string `json:"status,omitempty"`
	Content *string `json:"content,omitempty"`
}

// CustomMessagesErrorReviewMessageDTO configures failure notification.
type CustomMessagesErrorReviewMessageDTO struct {
	Status  *string `json:"status,omitempty"`
	Content *string `json:"content,omitempty"`
}

// CustomMessagesDTO wraps all custom bot message templates.
type CustomMessagesDTO struct {
	GlobalSettings     *CustomMessagesGlobalSettingsDTO     `json:"globalSettings,omitempty"`
	StartReviewMessage *CustomMessagesStartReviewMessageDTO `json:"startReviewMessage,omitempty"`
	EndReviewMessage   *CustomMessagesEndReviewMessageDTO   `json:"endReviewMessage,omitempty"`
	ErrorReviewMessage *CustomMessagesErrorReviewMessageDTO `json:"errorReviewMessage,omitempty"`
}

// DrixyKnowledgeApprovalDTO governs automated knowledge learning gate.
type DrixyKnowledgeApprovalDTO struct {
	Enabled bool `json:"enabled"`
}

// LinkedRepositoryDTO configures cross-repository review context.
type LinkedRepositoryDTO struct {
	Repository   string  `json:"repository"`
	Instructions *string `json:"instructions,omitempty"`
	Ref          *string `json:"ref,omitempty"`
}

// CodeReviewConfigWithoutLLMProviderDTO contains full code review settings.
type CodeReviewConfigWithoutLLMProviderDTO struct {
	ID                                     *string                     `json:"id,omitempty"`
	Name                                   *string                     `json:"name,omitempty"`
	Path                                   *string                     `json:"path,omitempty"`
	IsSelected                             *bool                       `json:"isSelected,omitempty"`
	IgnorePaths                            []string                    `json:"ignorePaths,omitempty"`
	ReviewOptions                          *ReviewOptionsDTO           `json:"reviewOptions,omitempty"`
	IgnoredTitleKeywords                   []string                    `json:"ignoredTitleKeywords,omitempty"`
	BaseBranches                           []string                    `json:"baseBranches,omitempty"`
	AutomatedReviewActive                  *bool                       `json:"automatedReviewActive,omitempty"`
	ShowStatusFeedback                     *bool                       `json:"showStatusFeedback,omitempty"`
	Summary                                *SummaryConfigDTO           `json:"summary,omitempty"`
	SuggestionControl                      *SuggestionControlConfigDTO `json:"suggestionControl,omitempty"`
	PullRequestApprovalActive              *bool                       `json:"pullRequestApprovalActive,omitempty"`
	ScanDrixConfigFileOverridesWebPreferences *bool                   `json:"scandrixConfigFileOverridesWebPreferences,omitempty"`
	IsRequestChangesActive                 *bool                       `json:"isRequestChangesActive,omitempty"`
	IDERulesSyncEnabled                    *bool                       `json:"ideRulesSyncEnabled,omitempty"`
	IDESyncDisableAction                   *string                     `json:"ideSyncDisableAction,omitempty"` // 'keep' | 'pause' | 'delete'
	DrixyRulesGeneratorEnabled             *bool                       `json:"drixyRulesGeneratorEnabled,omitempty"`
	DrixyLearningExcludedReviewers         []string                    `json:"drixyLearningExcludedReviewers,omitempty"`
	DrixyKnowledgeApproval                 *DrixyKnowledgeApprovalDTO  `json:"drixyKnowledgeApproval,omitempty"`
	ReviewCadence                          *ReviewCadenceDTO           `json:"reviewCadence,omitempty"`
	RunOnDraft                             *bool                       `json:"runOnDraft,omitempty"`
	CodeReviewVersion                      *string                     `json:"codeReviewVersion,omitempty"`
	V2PromptOverrides                      *V2PromptOverridesDTO       `json:"v2PromptOverrides,omitempty"`
	ContextReferenceID                     *string                     `json:"contextReferenceId,omitempty"`
	ContextRequirementsHash                *string                     `json:"contextRequirementsHash,omitempty"`
	CustomMessages                         *CustomMessagesDTO          `json:"customMessages,omitempty"`
	EnableCommittableSuggestions           *bool                       `json:"enableCommittableSuggestions,omitempty"`
	BYOKModel                              *string                     `json:"byokModel,omitempty"`
	BYOKModelID                            *string                     `json:"byokModelId,omitempty"`
	LinkedRepositories                     []LinkedRepositoryDTO       `json:"linkedRepositories,omitempty"`
}

// OrganizationAndTeamDataDTO conveys tenant context for parameter operations.
type OrganizationAndTeamDataDTO struct {
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	TeamID         *uuid.UUID `json:"teamId,omitempty"`
	TeamMemberID   *uuid.UUID `json:"teamMemberId,omitempty"`
	Provider       *string    `json:"provider,omitempty"`
	ProviderID     *string    `json:"providerId,omitempty"`
	WorkspaceID    *uuid.UUID `json:"workspaceId,omitempty"`
}

// CreateOrUpdateCodeReviewParameterDTO represents requests to update code review parameters.
type CreateOrUpdateCodeReviewParameterDTO struct {
	OrganizationAndTeamData OrganizationAndTeamDataDTO            `json:"organizationAndTeamData"`
	ConfigValue             CodeReviewConfigWithoutLLMProviderDTO `json:"configValue"`
	RepositoryID            *string                               `json:"repositoryId,omitempty"`
	DirectoryID             *string                               `json:"directoryId,omitempty"`
	DirectoryPath           *string                               `json:"directoryPath,omitempty"`
	DirectoryPaths          []string                              `json:"directoryPaths,omitempty"`
}

// DeleteRepositoryCodeReviewParameterDTO represents requests to remove repo-specific review overrides.
type DeleteRepositoryCodeReviewParameterDTO struct {
	TeamID         uuid.UUID `json:"teamId"`
	RepositoryID   string    `json:"repositoryId"`
	DirectoryID    *string   `json:"directoryId,omitempty"`
	FolderID       *string   `json:"folderId,omitempty"`
	DirectoryPath  *string   `json:"directoryPath,omitempty"`
	DirectoryPaths []string  `json:"directoryPaths,omitempty"`
}

// TeamIDQueryDTO parses teamId query parameter.
type TeamIDQueryDTO struct {
	TeamID uuid.UUID `json:"teamId"`
}

