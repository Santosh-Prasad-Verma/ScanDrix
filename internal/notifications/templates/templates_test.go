package templates_test

import (
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/notifications/templates"
)

func TestRenderSubscriptionWelcome(t *testing.T) {
	models := []string{"claude-sonnet-5", "gpt-5.6-terra", "gemini-3.7-flash"}
	subj, body := templates.RenderSubscriptionWelcome("Alice Smith", "Acme Corp", "TEAM", 10000000, models, "https://app.scandrix.dev")

	if !strings.Contains(subj, "Pro Team Plan") {
		t.Errorf("expected subject to contain Pro Team Plan, got %s", subj)
	}
	if !strings.Contains(body, "10,000,000") {
		t.Errorf("expected body to contain formatted 10,000,000 tokens")
	}
	if !strings.Contains(body, "claude-sonnet-5") {
		t.Errorf("expected body to list claude-sonnet-5")
	}
	if !strings.Contains(body, "Acme Corp") {
		t.Errorf("expected body to mention Acme Corp")
	}
}

func TestRenderInvoiceReceipt(t *testing.T) {
	inv := templates.InvoiceDetails{
		InvoiceNumber:    "INV-2026-998877",
		RecipientName:    "Bob Jones",
		RecipientEmail:   "bob@acme.com",
		OrganizationName: "Acme Labs",
		PlanTier:         "Pro Team",
		AmountFormatted:  "₹2,499.00 INR",
		OrderID:          "order_12345",
		PaymentID:        "pay_67890",
		PaymentProvider:  "Razorpay",
		BillingDate:      time.Now(),
		NextBillingDate:  time.Now().AddDate(0, 1, 0),
		BillingPortalURL: "https://app.scandrix.dev/billing",
	}

	subj, body := templates.RenderInvoiceReceipt(inv)

	if !strings.Contains(subj, "INV-2026-998877") {
		t.Errorf("expected subject to contain invoice number, got %s", subj)
	}
	if !strings.Contains(body, "₹2,499.00 INR") {
		t.Errorf("expected body to contain formatted amount")
	}
	if !strings.Contains(body, "pay_67890") {
		t.Errorf("expected body to contain payment ID")
	}
	if !strings.Contains(body, "PAID") {
		t.Errorf("expected body to have PAID status")
	}
}

func TestRenderPaymentFailed(t *testing.T) {
	subj, body := templates.RenderPaymentFailed("Bob", "Acme", "Pro Team", "order_fail_1", "Card limit exceeded", "https://app.scandrix.dev/billing")
	if !strings.Contains(subj, "Payment failed") {
		t.Errorf("expected subject to contain Payment failed, got %s", subj)
	}
	if !strings.Contains(body, "Card limit exceeded") {
		t.Errorf("expected body to contain failure reason")
	}
}

func TestRenderSpendLimitAlert(t *testing.T) {
	subj, body := templates.RenderSpendLimitAlert("Bob", "Acme", 80, 8000000, 10000000, "https://app.scandrix.dev/billing")
	if !strings.Contains(subj, "80%") {
		t.Errorf("expected subject to contain 80%%, got %s", subj)
	}
	if !strings.Contains(body, "8,000,000") {
		t.Errorf("expected body to contain used tokens")
	}
}

func TestRenderTeamInviteAndPasswordReset(t *testing.T) {
	subj1, body1 := templates.RenderTeamInvite("Alice", "charlie@acme.com", "Acme", "Security Lead", "https://app.scandrix.dev/invite")
	if !strings.Contains(subj1, "Alice invited you") {
		t.Errorf("expected invite subject")
	}
	if !strings.Contains(body1, "Security Lead") {
		t.Errorf("expected role in body")
	}

	subj2, body2 := templates.RenderPasswordReset("Charlie", "https://app.scandrix.dev/reset")
	if !strings.Contains(subj2, "Reset your ScanDrix Password") {
		t.Errorf("expected reset subject")
	}
	if !strings.Contains(body2, "Reset Password") {
		t.Errorf("expected reset button")
	}
}

func TestRenderEmailVerification(t *testing.T) {
	subj, body := templates.RenderEmailVerification("Tarun", "https://app.scandrix.dev/confirm-email?token=xyz123")
	if !strings.Contains(subj, "Verify your email to activate ScanDrix") {
		t.Errorf("unexpected verification subject: %s", subj)
	}
	if !strings.Contains(body, "Tarun") {
		t.Errorf("expected body to contain user's name")
	}
	if !strings.Contains(body, "Verify Email Address") {
		t.Errorf("expected body to contain CTA text")
	}
	if !strings.Contains(body, "https://app.scandrix.dev/confirm-email?token=xyz123") {
		t.Errorf("expected body to contain confirmation URL")
	}
}

func TestRenderNewUserWelcome(t *testing.T) {
	subj, body := templates.RenderNewUserWelcome("Tarun", "https://app.scandrix.dev/dashboard")
	if !strings.Contains(subj, "Welcome to ScanDrix") {
		t.Errorf("unexpected welcome subject: %s", subj)
	}
	if !strings.Contains(body, "Tarun") {
		t.Errorf("expected body to contain recipient name")
	}
	if !strings.Contains(body, "Quickstart Guide") {
		t.Errorf("expected body to contain quickstart guide")
	}
	if !strings.Contains(body, "Open Developer Dashboard") {
		t.Errorf("expected body to contain CTA button")
	}
}

func TestRenderPasswordResetWithDetails(t *testing.T) {
	d := templates.PasswordResetDetails{
		RecipientName:  "Alex Rivera",
		ResetURL:       "https://scandrix.dev/reset-password?token=pwd_123",
		Device:         "macOS 15.3 · Chrome 124",
		IPAddress:      "198.51.100.42",
		Location:       "San Francisco, CA, US",
		RequestedAt:    time.Now(),
		LockAccountURL: "https://scandrix.dev/account/lock?token=sec_lock_99",
	}

	subj, body := templates.RenderPasswordResetWithDetails(d)
	if !strings.Contains(subj, "Reset your ScanDrix Password") {
		t.Errorf("unexpected reset subject: %s", subj)
	}
	if !strings.Contains(body, "15 minutes") {
		t.Errorf("expected 15 minutes expiration warning in body")
	}
	if !strings.Contains(body, "198.51.100.42") {
		t.Errorf("expected IP address in body")
	}
	if !strings.Contains(body, "lock your account immediately") {
		t.Errorf("expected lock account link in body")
	}
}

func TestRenderNewDeviceLogin(t *testing.T) {
	d := templates.NewDeviceLoginDetails{
		RecipientName:  "Alex Rivera",
		Device:         "Linux x86_64 · Firefox 131.0",
		IPAddress:      "194.26.29.112",
		Location:       "Frankfurt, Germany",
		LoginTime:      time.Now(),
		LockAccountURL: "https://scandrix.dev/account/lock?token=tok_456",
		ActivityURL:    "https://scandrix.dev/cockpit/security",
	}

	subj, body := templates.RenderNewDeviceLogin(d)
	if !strings.Contains(subj, "Frankfurt, Germany") {
		t.Errorf("expected location in subject: %s", subj)
	}
	if !strings.Contains(body, "194.26.29.112") {
		t.Errorf("expected IP address in body")
	}
	if !strings.Contains(body, "Lock Account Immediately") {
		t.Errorf("expected lock account CTA")
	}
	if !strings.Contains(body, "Review Active Sessions") {
		t.Errorf("expected active sessions link")
	}
}

func TestRenderTeamAPIKeyCreated(t *testing.T) {
	d := templates.TeamAPIKeyCreatedDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		KeyName:       "github-actions-ci-pipeline",
		KeyPrefix:     "scandrix_live_9f82...3e71",
		CreatedBy:     "Alex Rivera (alex@acme.dev)",
		Scopes:        []string{"ci:scan", "pr:review", "rules:read"},
		CreatedAt:     time.Now(),
		ManageKeysURL: "https://scandrix.dev/cockpit/settings/api-keys",
	}

	subj, body := templates.RenderTeamAPIKeyCreated(d)
	if !strings.Contains(subj, "New Team API Key Created for Acme Engineering") {
		t.Errorf("unexpected API key subject: %s", subj)
	}
	if !strings.Contains(body, "scandrix_live_9f82...3e71") {
		t.Errorf("expected scandrix_live_ token prefix in body")
	}
	if !strings.Contains(body, "github-actions-ci-pipeline") {
		t.Errorf("expected key name in body")
	}
	if !strings.Contains(body, "Manage Workspace API Keys") {
		t.Errorf("expected manage keys CTA")
	}
}

func TestRenderCriticalVulnerabilityAlert(t *testing.T) {
	d := templates.CriticalVulnerabilityAlertDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		Repository:    "acme/auth-service",
		BranchOrPR:    "PR #142 (fix/auth-tokens)",
		Severity:      "CRITICAL",
		FindingTitle:  "CWE-89: SQL Injection Vulnerability in User Query",
		RuleID:        "scandrix-sec-sql-004",
		FilePath:      "internal/db/users.go",
		LineNumber:    74,
		Description:   "Unsanitized user parameter is concatenated into raw query.",
		Remediation:   "Use parameterized query placeholders.",
		FindingURL:    "https://scandrix.dev/cockpit/findings/fnd_123",
		SourceCodeURL: "https://github.com/acme/auth-service/blob/main/internal/db/users.go#L74",
	}

	subj, body := templates.RenderCriticalVulnerabilityAlert(d)
	if !strings.Contains(subj, "[CRITICAL]") || !strings.Contains(subj, "acme/auth-service") {
		t.Errorf("unexpected vulnerability alert subject: %s", subj)
	}
	if !strings.Contains(body, "internal/db/users.go:74") {
		t.Errorf("expected file and line in body")
	}
	if !strings.Contains(body, "Drixy Recommended Fix") {
		t.Errorf("expected Drixy fix recommendation in body")
	}
	if !strings.Contains(body, "View Finding in ScanDrix") {
		t.Errorf("expected view finding CTA")
	}
}

func TestRenderWeeklyScanDigest(t *testing.T) {
	d := templates.WeeklyScanDigestDetails{
		RecipientName:          "Alex Rivera",
		OrgName:                "Acme Engineering",
		WeekDateRange:          "Oct 01 - Oct 07, 2026",
		TotalPRsReviewed:       48,
		VulnerabilitiesBlocked: 14,
		CriticalBlocked:        3,
		HoursSaved:             18.5,
		HealthScore:            98,
		TopRepos: []templates.DigestRepoItem{
			{Repository: "acme/auth-service", PRsScanned: 24, IssuesBlocked: 7, HealthGrade: "A+"},
			{Repository: "acme/web-app", PRsScanned: 18, IssuesBlocked: 5, HealthGrade: "A"},
		},
		DashboardURL: "https://scandrix.dev/cockpit",
	}

	subj, body := templates.RenderWeeklyScanDigest(d)
	if !strings.Contains(subj, "Weekly Security Digest: Acme Engineering") {
		t.Errorf("unexpected weekly digest subject: %s", subj)
	}
	if !strings.Contains(body, "48") || !strings.Contains(body, "PRs Reviewed by Drixy") {
		t.Errorf("expected PRs reviewed metric in body")
	}
	if !strings.Contains(body, "18.5 hrs") {
		t.Errorf("expected hours saved metric in body")
	}
	if !strings.Contains(body, "acme/auth-service") {
		t.Errorf("expected top repository listed in body")
	}
}

func TestRenderPRReviewCompleted(t *testing.T) {
	d := templates.PRReviewCompletedDetails{
		RecipientName:    "Sarah Chen",
		OrgName:          "Acme Engineering",
		RepoName:         "acme/payment-api",
		PRNumber:         189,
		PRTitle:          "feat: implement OAuth2 PKCE token exchange",
		Author:           "Sarah Chen",
		Verdict:          "Changes Requested",
		CriticalCount:    1,
		HighCount:        0,
		SuggestionsCount: 3,
		DrixyNotes:       "Drixy identified a missing state validation check on the OAuth callback.",
		ReviewURL:        "https://github.com/acme/payment-api/pull/189#pullrequestreview-99",
	}

	subj, body := templates.RenderPRReviewCompleted(d)
	if !strings.Contains(subj, "Drixy Review: Changes Requested on acme/payment-api #189") {
		t.Errorf("unexpected PR review subject: %s", subj)
	}
	if !strings.Contains(body, "CHANGES REQUESTED") {
		t.Errorf("expected changes requested badge in body")
	}
	if !strings.Contains(body, "1 Critical") {
		t.Errorf("expected critical count in body")
	}
	if !strings.Contains(body, "View Review on Pull Request") {
		t.Errorf("expected review CTA in body")
	}
}

func TestRenderTrialExpiring(t *testing.T) {
	d := templates.TrialExpiringDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		DaysRemaining: 3,
		TrialEndDate:  time.Now().AddDate(0, 0, 3),
		PRsScanned:    64,
		IssuesBlocked: 9,
		HoursSaved:    14.2,
		UpgradeURL:    "https://scandrix.dev/cockpit/billing/upgrade",
	}

	subj, body := templates.RenderTrialExpiring(d)
	if !strings.Contains(subj, "3 Days Remaining") {
		t.Errorf("expected 3 days remaining in subject: %s", subj)
	}
	if !strings.Contains(body, "64") || !strings.Contains(body, "Pull Requests Scanned") {
		t.Errorf("expected PRs scanned stat in body")
	}
	if !strings.Contains(body, "Upgrade to ScanDrix Pro") {
		t.Errorf("expected upgrade CTA in body")
	}
}

func TestRenderUsageThresholdWarning(t *testing.T) {
	d80 := templates.UsageThresholdDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		PlanTier:      "Pro Team",
		PercentUsed:   80,
		UsedUnits:     40000000,
		LimitUnits:    50000000,
		UnitType:      "Tokens",
		ResetDate:     time.Now().AddDate(0, 0, 7),
		UpgradeURL:    "https://scandrix.dev/cockpit/billing",
	}

	subj80, body80 := templates.RenderUsageThresholdWarning(d80)
	if !strings.Contains(subj80, "80%") {
		t.Errorf("expected 80%% in subject: %s", subj80)
	}
	if !strings.Contains(body80, "40,000,000") {
		t.Errorf("expected formatted units in body: %s", body80)
	}

	d100 := templates.UsageThresholdDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		PlanTier:      "Pro Team",
		PercentUsed:   100,
		UsedUnits:     50000000,
		LimitUnits:    50000000,
		UnitType:      "Tokens",
		ResetDate:     time.Now().AddDate(0, 0, 7),
		UpgradeURL:    "https://scandrix.dev/cockpit/billing",
	}

	subj100, body100 := templates.RenderUsageThresholdWarning(d100)
	if !strings.Contains(subj100, "100%") || !strings.Contains(subj100, "Action Required") {
		t.Errorf("expected 100%% action required in subject: %s", subj100)
	}
	if !strings.Contains(body100, "100% Quota Exceeded") {
		t.Errorf("expected exceeded badge in body: %s", body100)
	}
}


