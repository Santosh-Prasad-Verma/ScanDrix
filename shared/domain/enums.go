package domain

// TenantPlan represents the subscription tier for a tenant.
type TenantPlan string

const (
	TenantPlanCommunity  TenantPlan = "COMMUNITY"
	TenantPlanTeam       TenantPlan = "TEAM"
	TenantPlanEnterprise TenantPlan = "ENTERPRISE"
)

// TenantStatus represents the operational status of a tenant.
type TenantStatus string

const (
	TenantStatusActive          TenantStatus = "ACTIVE"
	TenantStatusSuspended       TenantStatus = "SUSPENDED"
	TenantStatusPendingDeletion TenantStatus = "PENDING_DELETION"
)

// ScanType defines the mode of analysis.
type ScanType string

const (
	ScanTypeFull         ScanType = "FULL"
	ScanTypeDiffAware    ScanType = "DIFF_AWARE"
	ScanTypeSecurityOnly ScanType = "SECURITY_ONLY"
	ScanTypePerformance  ScanType = "PERFORMANCE"
)

// ScanStatus represents the lifecycle state of a scan.
type ScanStatus string

const (
	ScanStatusQueued       ScanStatus = "QUEUED"
	ScanStatusInitializing ScanStatus = "INITIALIZING"
	ScanStatusIngesting    ScanStatus = "INGESTING"
	ScanStatusScanning     ScanStatus = "SCANNING"
	ScanStatusVerifying    ScanStatus = "VERIFYING"
	ScanStatusCompleted    ScanStatus = "COMPLETED"
	ScanStatusFailed       ScanStatus = "FAILED"
	ScanStatusAborted      ScanStatus = "ABORTED"
)

// FindingSeverity indicates the risk level of a finding.
type FindingSeverity string

const (
	FindingSeverityCritical FindingSeverity = "CRITICAL"
	FindingSeverityHigh     FindingSeverity = "HIGH"
	FindingSeverityMedium   FindingSeverity = "MEDIUM"
	FindingSeverityLow      FindingSeverity = "LOW"
	FindingSeverityInfo     FindingSeverity = "INFO"
)

// FindingState indicates the verification lifecycle of a finding.
type FindingState string

const (
	FindingStateSuspected      FindingState = "SUSPECTED"
	FindingStateVerifiedProven FindingState = "VERIFIED_PROVEN"
	FindingStateResolved       FindingState = "RESOLVED"
	FindingStateFalsePositive  FindingState = "FALSE_POSITIVE"
	FindingStateAcceptedRisk   FindingState = "ACCEPTED_RISK"
)

// FindingCategory classifies the vulnerability or issue.
type FindingCategory string

const (
	FindingCategorySecurityVuln          FindingCategory = "SECURITY_VULN"
	FindingCategoryLogicBug              FindingCategory = "LOGIC_BUG"
	FindingCategorySecretLeak            FindingCategory = "SECRET_LEAK"
	FindingCategoryPerformanceBottleneck FindingCategory = "PERFORMANCE_BOTTLENECK"
	FindingCategoryBreakingAPIDiff       FindingCategory = "BREAKING_API_DIFF"
	FindingCategoryIaCMisconfig          FindingCategory = "IAC_MISCONFIG"
	FindingCategoryLicenseConflict       FindingCategory = "LICENSE_CONFLICT"
)

// ExecutionType defines the task executed in sandboxes or workers.
type ExecutionType string

const (
	ExecutionTypeASTParser          ExecutionType = "AST_PARSER"
	ExecutionTypeStaticSAST         ExecutionType = "STATIC_SAST"
	ExecutionTypeAIReasoning        ExecutionType = "AI_REASONING"
	ExecutionTypeSandboxTest        ExecutionType = "SANDBOX_TEST"
	ExecutionTypeMutationTest       ExecutionType = "MUTATION_TEST"
	ExecutionTypeDynamicDAST        ExecutionType = "DYNAMIC_DAST"
	ExecutionTypeK6LoadSurge        ExecutionType = "K6_LOAD_SURGE"
	ExecutionTypePatchVerification ExecutionType = "PATCH_VERIFICATION"
)

// SandboxTier defines the isolation environment.
type SandboxTier string

const (
	SandboxTierAGVisor            SandboxTier = "TIER_A_GVISOR"
	SandboxTierBFirecrackerMicroVM SandboxTier = "TIER_B_FIRECRACKER_MICROVM"
	SandboxTierCNetworkLab        SandboxTier = "TIER_C_NETWORK_LAB"
)

// ExecutionStatus defines the state of a sandbox run.
type ExecutionStatus string

const (
	ExecutionStatusQueued           ExecutionStatus = "QUEUED"
	ExecutionStatusRunning          ExecutionStatus = "RUNNING"
	ExecutionStatusPassed           ExecutionStatus = "PASSED"
	ExecutionStatusFailed           ExecutionStatus = "FAILED"
	ExecutionStatusTimedOut         ExecutionStatus = "TIMED_OUT"
	ExecutionStatusResourceExceeded ExecutionStatus = "RESOURCE_EXCEEDED"
)

// PatchStatus defines the state of an automated patch.
type PatchStatus string

const (
	PatchStatusProposed           PatchStatus = "PROPOSED"
	PatchStatusSandboxValidating  PatchStatus = "SANDBOX_VALIDATING"
	PatchStatusVerifiedPassing    PatchStatus = "VERIFIED_PASSING"
	PatchStatusVerificationFailed PatchStatus = "VERIFICATION_FAILED"
	PatchStatusPROpened           PatchStatus = "PR_OPENED"
	PatchStatusMerged             PatchStatus = "MERGED"
	PatchStatusRejected           PatchStatus = "REJECTED"
)
