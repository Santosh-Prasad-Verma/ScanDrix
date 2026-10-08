package catalog

import (
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// CatalogIcon represents an icon identifier for frontend rendering.
type CatalogIcon string

const (
	IconBell        CatalogIcon = "bell"
	IconShieldAlert CatalogIcon = "shield-alert"
	IconZap         CatalogIcon = "zap"
	IconInfo        CatalogIcon = "info"
	IconCreditCard  CatalogIcon = "credit-card"
)

// EventDefaults defines static configuration metadata for an event.
type EventDefaults struct {
	Criticality     enums.Criticality
	Category        string
	Label           string
	DefaultChannels []enums.Channel
	Icon            CatalogIcon
	PageSeverity    bool
	ActionLabel     string
	DefaultRoles    []string
}

// RoleWildcard represents the "All Roles" wildcard in routing rules.
const RoleWildcard = "*"

// Standard roles supported in notification routing.
const (
	RoleOwner          = "owner"
	RoleBillingManager = "billing_manager"
	RoleRepoAdmin      = "repo_admin"
	RoleContributor    = "contributor"
	RoleAdmin          = "admin"
	RoleMaintainer     = "maintainer"
	RoleReviewer       = "reviewer"
	RoleViewer         = "viewer"
)

// EventDefaultsMap maps each event to its static defaults.
var EventDefaultsMap = map[Event]EventDefaults{
	EventAuthEmailConfirmation: {
		Criticality:     enums.CriticalitySystem,
		Category:        "auth",
		Label:           "Email Confirmation",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconShieldAlert,
	},
	EventAuthForgotPassword: {
		Criticality:     enums.CriticalitySystem,
		Category:        "auth",
		Label:           "Forgot Password",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconShieldAlert,
	},
	EventAuthNewDeviceLogin: {
		Criticality:     enums.CriticalitySystem,
		Category:        "auth",
		Label:           "New Device Login Alert",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconShieldAlert,
	},
	EventAuthApiKeyCreated: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "auth",
		Label:           "Team API Key Created",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconShieldAlert,
	},
	EventCriticalVulnerability: {
		Criticality:     enums.CriticalityCritical,
		Category:        "security",
		Label:           "Critical Vulnerability Alert",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp, enums.ChannelSlack},
		Icon:            IconShieldAlert,
		PageSeverity:    true,
	},
	EventReviewCompleted: {
		Criticality:     enums.CriticalityInformational,
		Category:        "review",
		Label:           "Pull Request Review Completed",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconZap,
	},
	EventUsageThresholdWarning: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "spend_limit",
		Label:           "Usage Threshold Warning",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconCreditCard,
	},
	EventTeamMemberInvited: {
		Criticality:     enums.CriticalitySystem,
		Category:        "team",
		Label:           "Team Invite",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconZap,
	},
	EventDrixyRulesGenerated: {
		Criticality:     enums.CriticalityInformational,
		Category:        "drixy_rules",
		Label:           "Drixy Rules Generated",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconBell,
	},
	EventSSODomainVerification: {
		Criticality:     enums.CriticalitySystem,
		Category:        "sso",
		Label:           "SSO Domain Verification",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconShieldAlert,
	},
	EventRepoReport: {
		Criticality:     enums.CriticalityInformational,
		Category:        "cockpit",
		Label:           "Repo Report",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconInfo,
	},
	EventOrgReport: {
		Criticality:     enums.CriticalityInformational,
		Category:        "cockpit",
		Label:           "Org Report",
		DefaultChannels: []enums.Channel{enums.ChannelEmail},
		Icon:            IconInfo,
	},
	EventOrgMemberRemoved: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "team",
		Label:           "Member Removed",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconZap,
	},
	EventOrgRoleChanged: {
		Criticality:     enums.CriticalityInformational,
		Category:        "team",
		Label:           "Role Changed",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconZap,
		DefaultRoles:    []string{RoleOwner, RoleAdmin},
	},
	EventIDERulesSynced: {
		Criticality:     enums.CriticalityInformational,
		Category:        "drixy_rules",
		Label:           "IDE Rules Synced",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconBell,
	},
	EventIDERulesSyncFailed: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "drixy_rules",
		Label:           "IDE Rule Sync Failed",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconShieldAlert,
		DefaultRoles:    []string{RoleOwner, RoleAdmin},
	},
	EventReviewAutoApproved: {
		Criticality:     enums.CriticalityInformational,
		Category:        "review",
		Label:           "Pull Request Auto-Approved",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconBell,
	},
	EventReviewFailed: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "review",
		Label:           "Code Review Failed",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconShieldAlert,
		DefaultRoles:    []string{RoleOwner, RoleAdmin},
	},
	EventReviewSkippedNoLicense: {
		Criticality:     enums.CriticalityInformational,
		Category:        "review",
		Label:           "Review Skipped (No License)",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconInfo,
	},
	EventBillingPaymentFailed: {
		Criticality:     enums.CriticalityCritical,
		Category:        "billing",
		Label:           "Payment Failed",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconCreditCard,
		PageSeverity:    true,
		ActionLabel:     "Update payment",
		DefaultRoles:    []string{RoleOwner, RoleBillingManager, RoleAdmin},
	},
	EventBillingTrialExpiring: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "billing",
		Label:           "Trial Expiring",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconCreditCard,
		ActionLabel:     "Upgrade plan",
		DefaultRoles:    []string{RoleOwner, RoleBillingManager, RoleAdmin},
	},
	EventByokLlmErrorsThreshold: {
		Criticality:     enums.CriticalityCritical,
		Category:        "byok",
		Label:           "BYOK LLM Errors Exceeded Threshold",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconShieldAlert,
		PageSeverity:    true,
		DefaultRoles:    []string{RoleOwner, RoleAdmin},
	},
	EventSpendLimitThresholdReached: {
		Criticality:     enums.CriticalityInformational,
		Category:        "spend_limit",
		Label:           "Monthly Spend Limit Threshold Reached",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconCreditCard,
		DefaultRoles:    []string{RoleOwner, RoleAdmin},
	},
	EventSpendLimitExceededFinal: {
		Criticality:     enums.CriticalityCritical,
		Category:        "spend_limit",
		Label:           "Monthly Spend Limit Exceeded",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconCreditCard,
		PageSeverity:    true,
		DefaultRoles:    []string{RoleOwner, RoleAdmin},
	},
	EventRuleFileReferencesInvalid: {
		Criticality:     enums.CriticalityTransactional,
		Category:        "drixy_rules",
		Label:           "Rule File References Invalid",
		DefaultChannels: []enums.Channel{enums.ChannelEmail, enums.ChannelInApp},
		Icon:            IconShieldAlert,
	},
}

// EventCategories lists all category identifiers.
var EventCategories = []string{
	"auth",
	"team",
	"drixy_rules",
	"sso",
	"cockpit",
	"billing",
	"review",
	"byok",
	"spend_limit",
}

// ChannelLabels maps channel codes to UI display labels.
var ChannelLabels = map[enums.Channel]string{
	enums.ChannelEmail:   "Email",
	enums.ChannelInApp:   "In-App",
	enums.ChannelSlack:   "Slack",
	enums.ChannelDiscord: "Discord",
	enums.ChannelWebhook: "Webhook",
}

// CriticalityLabels maps criticality levels to UI display labels.
var CriticalityLabels = map[enums.Criticality]string{
	enums.CriticalitySystem:        "System",
	enums.CriticalityCritical:      "Critical",
	enums.CriticalityTransactional: "Transactional",
	enums.CriticalityInformational: "Informational",
}

// CategoryLabels maps category codes to UI display labels.
var CategoryLabels = map[string]string{
	"auth":        "Auth",
	"team":        "Team",
	"drixy_rules": "Drixy Rules",
	"sso":         "SSO",
	"cockpit":     "Cockpit",
	"billing":     "Billing",
	"review":      "Code Review",
	"byok":        "BYOK",
	"spend_limit": "Spend Limit",
}

// RoleLabels maps role codes to UI display labels.
var RoleLabels = map[string]string{
	RoleWildcard:       "All Roles",
	RoleOwner:          "Owner",
	RoleBillingManager: "Billing Manager",
	RoleRepoAdmin:      "Repo Admin",
	RoleContributor:    "Contributor",
	RoleAdmin:          "Admin",
	RoleMaintainer:     "Maintainer",
	RoleReviewer:       "Reviewer",
	RoleViewer:         "Viewer",
}
