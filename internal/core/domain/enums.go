package domain

// JobStatus mirrors ScanDrix JobStatus enum.
type JobStatus string

const (
	JobStatusPending         JobStatus = "PENDING"
	JobStatusProcessing      JobStatus = "PROCESSING"
	JobStatusCompleted       JobStatus = "COMPLETED"
	JobStatusFailed          JobStatus = "FAILED"
	JobStatusWaitingForEvent JobStatus = "WAITING_FOR_EVENT"
	JobStatusCancelled       JobStatus = "CANCELLED"
)

// ErrorClassification mirrors ScanDrix ErrorClassification enum.
type ErrorClassification string

const (
	ErrorClassificationRetryable    ErrorClassification = "RETRYABLE"
	ErrorClassificationNonRetryable ErrorClassification = "NON_RETRYABLE"
	ErrorClassificationCircuitOpen  ErrorClassification = "CIRCUIT_OPEN"
	ErrorClassificationPermanent    ErrorClassification = "PERMANENT"
	ErrorClassificationRateLimited  ErrorClassification = "RATE_LIMITED"
)

// WorkflowType mirrors ScanDrix WorkflowType enum.
type WorkflowType string

const (
	WorkflowTypeCodeReview                  WorkflowType = "CODE_REVIEW"
	WorkflowTypeCLICodeReview               WorkflowType = "CLI_CODE_REVIEW"
	WorkflowTypeCronCheckPRApproval         WorkflowType = "CRON_CHECK_PR_APPROVAL"
	WorkflowTypeCronDrixyLearning           WorkflowType = "CRON_DRIXY_LEARNING"
	WorkflowTypeCronCodeReviewFeedback      WorkflowType = "CRON_CODE_REVIEW_FEEDBACK"
	WorkflowTypeWebhookProcessing           WorkflowType = "WEBHOOK_PROCESSING"
	WorkflowTypeCheckSuggestionVerification WorkflowType = "CHECK_SUGGESTION_IMPLEMENTATION"
	WorkflowTypeASTGraphBuild               WorkflowType = "AST_GRAPH_BUILD"
	WorkflowTypeASTGraphIncremental         WorkflowType = "AST_GRAPH_INCREMENTAL"
)

// HandlerType mirrors ScanDrix HandlerType enum.
type HandlerType string

const (
	HandlerTypePipelineSync    HandlerType = "PIPELINE_SYNC"
	HandlerTypePipelineAsync   HandlerType = "PIPELINE_ASYNC"
	HandlerTypeSimpleFunction  HandlerType = "SIMPLE_FUNCTION"
	HandlerTypeWebhookRaw      HandlerType = "WEBHOOK_RAW"
)

// HeavyStageEventType mirrors ScanDrix EventType enum for heavy stage completion.
type HeavyStageEventType string

const (
	EventTypeASTAnalysisCompleted     HeavyStageEventType = "ast.task.completed"
	EventTypePRLevelReviewCompleted   HeavyStageEventType = "pr.level.review.completed"
	EventTypeFilesReviewCompleted     HeavyStageEventType = "files.review.completed"
	EventTypeLLMAnalysisCompleted     HeavyStageEventType = "llm.analysis.completed"
)

// OutboxStatus mirrors ScanDrix OutboxStatus enum.
type OutboxStatus string

const (
	OutboxStatusReady      OutboxStatus = "READY"
	OutboxStatusProcessing OutboxStatus = "PROCESSING"
	OutboxStatusSent       OutboxStatus = "SENT"
	OutboxStatusFailed     OutboxStatus = "FAILED"
)

// InboxStatus mirrors ScanDrix InboxStatus enum.
type InboxStatus string

const (
	InboxStatusReady      InboxStatus = "READY"
	InboxStatusProcessing InboxStatus = "PROCESSING"
	InboxStatusProcessed  InboxStatus = "PROCESSED"
	InboxStatusFailed     InboxStatus = "FAILED"
)

// ParametersKey mirrors ScanDrix ParametersKey enum.
type ParametersKey string

const (
	ParametersKeyCodeReviewConfig       ParametersKey = "code_review_config"
	ParametersKeyPlatformConfigs        ParametersKey = "platform_configs"
	ParametersKeyLanguageConfig         ParametersKey = "language_config"
	ParametersKeyIssueCreationConfig    ParametersKey = "issue_creation_config"
	ParametersKeyCentralizedConfig      ParametersKey = "centralized_config"
)

// OrganizationParametersKey mirrors ScanDrix OrganizationParametersKey enum.
type OrganizationParametersKey string

const (
	OrgParamCategoryWorkitemTypes        OrganizationParametersKey = "category_workitems_type"
	OrgParamTimezoneConfig               OrganizationParametersKey = "timezone_config"
	OrgParamReviewModeConfig             OrganizationParametersKey = "review_mode_config"
	OrgParamDrixyFineTuningConfig        OrganizationParametersKey = "drixy_fine_tuning_config"
	OrgParamAutoJoinConfig               OrganizationParametersKey = "auto_join_config"
	OrgParamBYOKConfig                   OrganizationParametersKey = "byok_config"
	OrgParamCockpitMetricsVisibility     OrganizationParametersKey = "cockpit_metrics_visibility"
	OrgParamAutoLicenseAssignment        OrganizationParametersKey = "auto_license_assignment"
	OrgParamCodeReviewPreset             OrganizationParametersKey = "code_review_preset"
	OrgParamLicenseKey                   OrganizationParametersKey = "license_key"
	OrgParamLicenseAssignedUsers         OrganizationParametersKey = "license_assigned_users"
	OrgParamFirstReviewAt                OrganizationParametersKey = "first_review_at"
	OrgParamSpendLimitConfig             OrganizationParametersKey = "spend_limit_config"
	OrgParamGlobalRulesSourceRepos       OrganizationParametersKey = "global_rules_source_repositories"
)

// GlobalParametersKey mirrors ScanDrix GlobalParametersKey enum.
type GlobalParametersKey string

const (
	GlobalParamDrixyFineTuningConfig GlobalParametersKey = "drixy_fine_tuning_config"
	GlobalParamCodeReviewMaxFiles    GlobalParametersKey = "code_review_max_files"
	GlobalParamIgnorePathsGlobal     GlobalParametersKey = "ignore_paths_global"
	GlobalParamIPE2B                 GlobalParametersKey = "ip_e2b"
	GlobalParamTelemetryState        GlobalParametersKey = "telemetry_state"
)

// IntegrationConfigKey mirrors ScanDrix IntegrationConfigKey enum.
type IntegrationConfigKey string

const (
	IntegrationConfigKeyColumnsMapping                  IntegrationConfigKey = "columns_mapping"
	IntegrationConfigKeyProjectManagementSetupConfig    IntegrationConfigKey = "project_management_setup_config"
	IntegrationConfigKeyRepositories                    IntegrationConfigKey = "repositories"
	IntegrationConfigKeyInstallationGithub              IntegrationConfigKey = "installation_github"
	IntegrationConfigKeyChannelInfo                     IntegrationConfigKey = "channel_info"
	IntegrationConfigKeyMSTeamsInstallationApp          IntegrationConfigKey = "msteams_installation_app"
	IntegrationConfigKeyWaitingColumns                  IntegrationConfigKey = "waiting_columns"
	IntegrationConfigKeyDoingColumn                     IntegrationConfigKey = "doing_column"
	IntegrationConfigKeyDailyCheckinSchedule            IntegrationConfigKey = "daily_checkin_schedule"
	IntegrationConfigKeyModuleWorkitemsTypes            IntegrationConfigKey = "module_workitems_types"
	IntegrationConfigKeyBugTypeIdentifiers              IntegrationConfigKey = "bug_type_identifier"
	IntegrationConfigKeyAutomationIssueAlertTime        IntegrationConfigKey = "automation_issue_alert_time"
	IntegrationConfigKeyTeamProjectManagementMethodology IntegrationConfigKey = "team_project_management_methodology"
	IntegrationConfigKeyCodeManagementPAT               IntegrationConfigKey = "code_management_pat"
	IntegrationConfigKeyUseJQLToViewBoard               IntegrationConfigKey = "use_jql_to_view_board"
)

// PlatformType mirrors ScanDrix PlatformType enum.
type PlatformType string

const (
	PlatformTypeInternal    PlatformType = "INTERNAL"
	PlatformTypeGitHub      PlatformType = "GITHUB"
	PlatformTypeGitLab      PlatformType = "GITLAB"
	PlatformTypeJira        PlatformType = "JIRA"
	PlatformTypeSlack       PlatformType = "SLACK"
	PlatformTypeNotion      PlatformType = "NOTION"
	PlatformTypeMSTeams     PlatformType = "MSTEAMS"
	PlatformTypeDiscord     PlatformType = "DISCORD"
	PlatformTypeAzureBoards PlatformType = "AZURE_BOARDS"
	PlatformTypeAzureRepos  PlatformType = "AZURE_REPOS"
	PlatformTypeScanDrixWeb PlatformType = "SCANDRIX_WEB"
	PlatformTypeBitbucket   PlatformType = "BITBUCKET"
	PlatformTypeForgejo     PlatformType = "FORGEJO"
)

// PullRequestState mirrors ScanDrix PullRequestState enum.
type PullRequestState string

const (
	PullRequestStateOpen   PullRequestState = "OPEN"
	PullRequestStateClosed PullRequestState = "CLOSED"
	PullRequestStateMerged PullRequestState = "MERGED"
)

// AutomationStatus mirrors ScanDrix AutomationStatus enum.
type AutomationStatus string

const (
	AutomationStatusPending AutomationStatus = "PENDING"
	AutomationStatusRunning AutomationStatus = "RUNNING"
	AutomationStatusSuccess AutomationStatus = "SUCCESS"
	AutomationStatusFailure AutomationStatus = "FAILURE"
	AutomationStatusSkipped AutomationStatus = "SKIPPED"
)

// PipelineErrorSeverity mirrors ScanDrix PipelineErrorSeverity enum.
type PipelineErrorSeverity string

const (
	PipelineSeverityLow      PipelineErrorSeverity = "LOW"
	PipelineSeverityMedium   PipelineErrorSeverity = "MEDIUM"
	PipelineSeverityHigh     PipelineErrorSeverity = "HIGH"
	PipelineSeverityCritical PipelineErrorSeverity = "CRITICAL"
)

// ProgrammingLanguage mirrors ScanDrix ProgrammingLanguage enum.
type ProgrammingLanguage string

const (
	LangTypeScript ProgrammingLanguage = "TYPESCRIPT"
	LangJavaScript ProgrammingLanguage = "JAVASCRIPT"
	LangPython     ProgrammingLanguage = "PYTHON"
	LangGo         ProgrammingLanguage = "GO"
	LangJava       ProgrammingLanguage = "JAVA"
	LangCPP        ProgrammingLanguage = "CPP"
	LangCSharp     ProgrammingLanguage = "CSHARP"
	LangRuby       ProgrammingLanguage = "RUBY"
	LangPHP        ProgrammingLanguage = "PHP"
	LangRust       ProgrammingLanguage = "RUST"
)

// LanguageValue represents supported human language codes for reviews.
type LanguageValue string

const (
	LanguageEnglish        LanguageValue = "en-US"
	LanguagePortugueseBR   LanguageValue = "pt-BR"
	LanguagePortuguesePT   LanguageValue = "pt-PT"
	LanguageSpanish        LanguageValue = "es-ES"
	LanguageFrench         LanguageValue = "fr-FR"
	LanguageGerman         LanguageValue = "de-DE"
	LanguageItalian        LanguageValue = "it-IT"
	LanguageDutch          LanguageValue = "nl-NL"
	LanguagePolish         LanguageValue = "pl-PL"
	LanguageRussian        LanguageValue = "ru-RU"
	LanguageArabic         LanguageValue = "ar-SA"
	LanguageChineseMainland LanguageValue = "zh-CN"
	LanguageHindi          LanguageValue = "hi-IN"
	LanguageJapanese       LanguageValue = "ja-JP"
	LanguageKorean         LanguageValue = "ko-KR"
	LanguageVietnamese     LanguageValue = "vi-VN"
	LanguageThai           LanguageValue = "th-TH"
	LanguageSwedish        LanguageValue = "sv-SE"
	LanguageFinnish        LanguageValue = "fi-FI"
	LanguageNorwegian      LanguageValue = "nb-NO"
	LanguageDanish         LanguageValue = "da-DK"
	LanguageCzech          LanguageValue = "cs-CZ"
	LanguageHungarian      LanguageValue = "hu-HU"
	LanguageUkrainian      LanguageValue = "uk-UA"
	LanguageTamil          LanguageValue = "ta-IN"
	LanguageTelugu         LanguageValue = "te-IN"
	LanguageHebrew         LanguageValue = "he-IL"
	LanguageTurkish        LanguageValue = "tr-TR"
	LanguageIndonesian     LanguageValue = "id-ID"
	LanguageMalay          LanguageValue = "ms-MY"
	LanguageGreek          LanguageValue = "el-GR"
	LanguageRomanian       LanguageValue = "ro-RO"
	LanguageBulgarian      LanguageValue = "bg-BG"
)

// Timezone represents supported system and notification timezones.
type Timezone string

const (
	TimezoneNewYork  Timezone = "America/New_York"
	TimezoneSaoPaulo Timezone = "America/Sao_Paulo"
	TimezoneUTC      Timezone = "UTC"
	TimezoneDefault  Timezone = "America/New_York"
)

// IntegrationCategory classifies integrations by architectural purpose.
type IntegrationCategory string

const (
	IntegrationCategoryCodeManagement    IntegrationCategory = "CODE_MANAGEMENT"
	IntegrationCategoryProjectManagement IntegrationCategory = "PROJECT_MANAGEMENT"
	IntegrationCategoryCommunication     IntegrationCategory = "COMMUNICATION"
)

// StatusCategoryAzureBoards maps Azure Boards work item statuses.
type StatusCategoryAzureBoards string

const (
	StatusCategoryIncoming   StatusCategoryAzureBoards = "incoming"
	StatusCategoryInProgress StatusCategoryAzureBoards = "inProgress"
	StatusCategoryOutgoing   StatusCategoryAzureBoards = "outgoing"
)

// AgentExecutionType identifies specialized autonomous agents.
type AgentExecutionType string

const (
	AgentExecutionCodeReview AgentExecutionType = "CodeReviewAgent"
)

// AutomationLevel specifies the organizational scope of an automation rule.
type AutomationLevel string

const (
	AutomationLevelOrganization AutomationLevel = "ORGANIZATION"
	AutomationLevelTeam         AutomationLevel = "TEAM"
)

// InstallationStatus tracks third-party SCM app installation state.
type InstallationStatus string

const (
	InstallationStatusPending InstallationStatus = "PENDING"
	InstallationStatusSuccess InstallationStatus = "SUCCESS"
)
