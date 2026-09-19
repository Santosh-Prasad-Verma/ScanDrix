package catalog

// Event identifies a specific notification event in the typed catalog.
type Event string

const (
	// Auth
	EventAuthEmailConfirmation Event = "auth.email_confirmation"
	EventAuthForgotPassword    Event = "auth.forgot_password"

	// Organization / Team
	EventTeamMemberInvited Event = "team.member_invited"
	EventOrgMemberRemoved  Event = "org.member_removed"
	EventOrgRoleChanged    Event = "org.role_changed"

	// Drixy Rules
	EventDrixyRulesGenerated        Event = "drixy_rules.generated"
	EventRuleFileReferencesInvalid Event = "rule.file_references_invalid"

	// IDE Rule Sync
	EventIDERulesSynced     Event = "ide.rules_synced"
	EventIDERulesSyncFailed Event = "ide.rules_sync_failed"

	// Code Review
	EventReviewAutoApproved     Event = "review.auto_approved"
	EventReviewFailed           Event = "review.failed"
	EventReviewSkippedNoLicense Event = "review.skipped_no_license"

	// SSO
	EventSSODomainVerification Event = "sso.domain_verification"

	// Cockpit Reports
	EventRepoReport Event = "cockpit.repo_report"
	EventOrgReport  Event = "cockpit.org_report"

	// Billing
	EventBillingPaymentFailed Event = "billing.payment_failed"
	EventBillingTrialExpiring Event = "billing.trial_expiring"

	// BYOK
	EventByokLlmErrorsThreshold Event = "byok.llm_errors_threshold"

	// Spend Limit
	EventSpendLimitThresholdReached Event = "spend_limit.threshold_reached"
	EventSpendLimitExceededFinal    Event = "spend_limit.exceeded_final"
)

// AuthEmailConfirmationPayload describes payload for auth.email_confirmation.
type AuthEmailConfirmationPayload struct {
	Token                   string                 `json:"token"`
	Email                   string                 `json:"email"`
	OrganizationName        string                 `json:"organizationName"`
	OrganizationAndTeamData map[string]interface{} `json:"organizationAndTeamData,omitempty"`
}

// AuthForgotPasswordPayload describes payload for auth.forgot_password.
type AuthForgotPasswordPayload struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

// TeamMemberInvitedPayload describes payload for team.member_invited.
type TeamMemberInvitedPayload struct {
	User         map[string]interface{} `json:"user"`
	InviterEmail string                 `json:"inviterEmail"`
	InviteLink   string                 `json:"inviteLink"`
}

// DrixyRulesGeneratedPayload describes payload for drixy_rules.generated.
type DrixyRulesGeneratedPayload struct {
	Users            []RecipientUserRef `json:"users"`
	Rules            []string           `json:"rules"`
	OrganizationName string             `json:"organizationName"`
}

// RecipientUserRef contains basic user info for multi-user notifications.
type RecipientUserRef struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// SSODomainVerificationPayload describes payload for sso.domain_verification.
type SSODomainVerificationPayload struct {
	Token            string `json:"token"`
	Email            string `json:"email"`
	OrganizationName string `json:"organizationName"`
	Domain           string `json:"domain"`
}

// ReportRecipient contains name and email for digest/cockpit reports.
type ReportRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// RepoReportPayload describes payload for cockpit.repo_report.
type RepoReportPayload struct {
	Recipient ReportRecipient        `json:"recipient"`
	Props     map[string]interface{} `json:"props"`
}

// OrgReportPayload describes payload for cockpit.org_report.
type OrgReportPayload struct {
	Recipient ReportRecipient        `json:"recipient"`
	Props     map[string]interface{} `json:"props"`
}

// OrgMemberRemovedPayload describes payload for org.member_removed.
type OrgMemberRemovedPayload struct {
	RemovedUser      RecipientUserRef `json:"removedUser"`
	RemovedBy        string           `json:"removedBy"`
	RemovedAt        string           `json:"removedAt"`
	OrganizationName string           `json:"organizationName"`
}

// OrgRoleChangedPayload describes payload for org.role_changed.
type OrgRoleChangedPayload struct {
	AffectedUserEmail string `json:"affectedUserEmail"`
	PreviousRole      string `json:"previousRole"`
	NewRole           string `json:"newRole"`
	ChangedBy         string `json:"changedBy"`
	OrganizationName  string `json:"organizationName"`
}

// IDERulesSyncedPayload describes payload for ide.rules_synced.
type IDERulesSyncedPayload struct {
	RepoName   string `json:"repoName"`
	RulesCount int    `json:"rulesCount"`
	SyncMode   string `json:"syncMode"` // "fast" | "full" | "changed-files"
}

// IDERulesSyncFailedPayload describes payload for ide.rules_sync_failed.
type IDERulesSyncFailedPayload struct {
	RepoName      string `json:"repoName"`
	Reason        string `json:"reason"`
	CorrelationID string `json:"correlationId"`
}

// ReviewAutoApprovedPayload describes payload for review.auto_approved.
type ReviewAutoApprovedPayload struct {
	PRURL      string `json:"prUrl"`
	RepoName   string `json:"repoName"`
	ApprovedAt string `json:"approvedAt"`
}

// ReviewFailedPayload describes payload for review.failed.
type ReviewFailedPayload struct {
	PRURL         string `json:"prUrl"`
	RepoName      string `json:"repoName"`
	Reason        string `json:"reason"`
	CorrelationID string `json:"correlationId"`
}

// ReviewSkippedNoLicensePayload describes payload for review.skipped_no_license.
type ReviewSkippedNoLicensePayload struct {
	PRURL          string `json:"prUrl"`
	RepoName       string `json:"repoName"`
	OwnerContact   string `json:"ownerContact,omitempty"`
	AuthorUsername string `json:"authorUsername,omitempty"`
}

// BillingPaymentFailedPayload describes payload for billing.payment_failed.
type BillingPaymentFailedPayload struct {
	Amount           int64  `json:"amount"` // in cents
	Currency         string `json:"currency"`
	FailureReason    string `json:"failureReason"`
	NextRetryAt      string `json:"nextRetryAt,omitempty"`
	UpdatePaymentURL string `json:"updatePaymentUrl,omitempty"`
}

// BillingTrialExpiringPayload describes payload for billing.trial_expiring.
type BillingTrialExpiringPayload struct {
	TrialEndsAt   string `json:"trialEndsAt"`
	DaysRemaining int    `json:"daysRemaining"`
	UpgradeURL    string `json:"upgradeUrl,omitempty"`
}

// ByokLlmErrorsThresholdPayload describes payload for byok.llm_errors_threshold.
type ByokLlmErrorsThresholdPayload struct {
	Provider    string `json:"provider"`
	ErrorCount  int    `json:"errorCount"`
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`
	SampleError string `json:"sampleError"`
}

// SpendLimitThresholdReachedPayload describes payload for spend_limit.threshold_reached.
type SpendLimitThresholdReachedPayload struct {
	Percentage      int     `json:"percentage"` // 50, 75, 90, 100
	MonthlyLimitUSD float64 `json:"monthlyLimitUsd"`
	SpentUSD        float64 `json:"spentUsd"`
	PeriodKey       string  `json:"periodKey"` // YYYY-MM
}

// SpendLimitExceededFinalPayload describes payload for spend_limit.exceeded_final.
type SpendLimitExceededFinalPayload struct {
	MonthlyLimitUSD float64 `json:"monthlyLimitUsd"`
	SpentUSD        float64 `json:"spentUsd"`
	PeriodKey       string  `json:"periodKey"` // YYYY-MM
}

// RuleReferenceIssue describes an individual broken file reference.
type RuleReferenceIssue struct {
	RuleID   string `json:"ruleId"`
	RuleName string `json:"ruleName"`
	FilePath string `json:"filePath"`
	Reason   string `json:"reason"`
}

// RuleFileReferencesInvalidPayload describes payload for rule.file_references_invalid.
type RuleFileReferencesInvalidPayload struct {
	Source       string               `json:"source"` // "ide" | "manual" | "auto_recheck"
	RepoName     string               `json:"repoName"`
	InvalidCount int                  `json:"invalidCount"`
	Issues       []RuleReferenceIssue `json:"issues"`
}
