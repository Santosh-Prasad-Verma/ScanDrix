package domain

// AutomationType defines specific automation capabilities supported by ScanDrix.
type AutomationType string

const (
	AutomationTeamProgress             AutomationType = "AutomationTeamProgress"
	AutomationInteractionMonitor       AutomationType = "AutomationInteractionMonitor"
	AutomationIssuesDetails            AutomationType = "AutomationIssuesDetails"
	AutomationImproveTask              AutomationType = "AutomationImproveTask"
	AutomationEnsureAssignees          AutomationType = "AutomationEnsureAssignees"
	AutomationCommitValidation         AutomationType = "AutomationCommitValidation"
	AutomationWipLimits                AutomationType = "AutomationWipLimits"
	AutomationWaitingConstraints       AutomationType = "AutomationWaitingConstraints"
	AutomationTaskBreakdown            AutomationType = "AutomationTaskBreakdown"
	AutomationUserRequestedBreakdown   AutomationType = "AutomationUserRequestedBreakdown"
	AutomationRetroactiveMovement      AutomationType = "AutomationRetroactiveMovement"
	AutomationDailyCheckin             AutomationType = "AutomationDailyCheckin"
	AutomationSprintRetro              AutomationType = "AutomationSprintRetro"
	AutomationExecutiveCheckin         AutomationType = "AutomationExecutiveCheckin"
	AutomationCodeReview               AutomationType = "AutomationCodeReview"
)

// AutomationTypeCategory groups automations by operational domain.
type AutomationTypeCategory string

const (
	CategoryCodeManagement AutomationTypeCategory = "CodeManagementAutomations"
)

// AutomationCategoryMapping maps categories to their member automation types.
var AutomationCategoryMapping = map[AutomationTypeCategory][]AutomationType{
	CategoryCodeManagement: {
		AutomationCodeReview,
	},
}

// AutomationLevel defines the organizational scope of an automation.
type AutomationLevel string

const (
	LevelOrganization AutomationLevel = "ORGANIZATION"
	LevelTeam         AutomationLevel = "TEAM"
	LevelUser         AutomationLevel = "USER"
)

// AutomationStatus represents the lifecycle state of an automation execution.
type AutomationStatus string

const (
	StatusPending      AutomationStatus = "pending"
	StatusInProgress   AutomationStatus = "in_progress"
	StatusSuccess      AutomationStatus = "success"
	StatusError        AutomationStatus = "error"
	StatusPartialError AutomationStatus = "partial_error"
	StatusSkipped      AutomationStatus = "skipped"
)

// AutomationMessage provides standard feedback messages for review automation stages.
type AutomationMessage string

const (
	MsgNoConfigInContext         AutomationMessage = "No code-review configuration found in the current context."
	MsgNoFilesAfterIgnore        AutomationMessage = "No files remain after applying ignore patterns."
	MsgTooManyFiles              AutomationMessage = "Too many files to analyze."
	MsgNoFilesInPR               AutomationMessage = "No changed files in this pull request."
	MsgFailedResolveConfig       AutomationMessage = "Unable to load or resolve the review configuration."
	MsgSkippedByBasicRules       AutomationMessage = "Skipped by baseline configuration rules."
	MsgProcessingManual          AutomationMessage = "Processing due to manual command."
	MsgProcessingAutomatic       AutomationMessage = "Processing in automatic mode."
	MsgFirstReviewManual         AutomationMessage = "Starting first review (manual mode)."
	MsgManualRequiredToStart     AutomationMessage = "Manual mode requires @drixy start-review."
	MsgFirstReviewAutoPause      AutomationMessage = "Starting first review (auto-pause mode)."
	MsgPRPausedNeedResume        AutomationMessage = "PR is paused — use @drixy start-review to resume."
	MsgPRPausedBurstPushes       AutomationMessage = "PR is paused due to multiple pushes in a short time window."
	MsgProcessingAutoPause       AutomationMessage = "Processing in auto-pause mode."
	MsgConfigValidationError     AutomationMessage = "Error during configuration validation."
	MsgNoNewCommitsSinceLast     AutomationMessage = "No new commits since the last run."
	MsgOnlyMergeCommitsSinceLast AutomationMessage = "Only merge commits since the last run."
	MsgProviderRateLimited       AutomationMessage = "The Git provider rate-limited the request; the review was not run."
	MsgUserIgnored               AutomationMessage = "User is ignored by configuration."
	MsgValidationFailed          AutomationMessage = "Prerequisites validation failed."
)

// CodeReviewExecutionTrigger indicates how a code review was initiated.
type CodeReviewExecutionTrigger string

const (
	TriggerAutomatic  CodeReviewExecutionTrigger = "AUTOMATIC"
	TriggerCommand    CodeReviewExecutionTrigger = "COMMAND"
	TriggerCommitPush CodeReviewExecutionTrigger = "COMMIT_PUSH"
)
