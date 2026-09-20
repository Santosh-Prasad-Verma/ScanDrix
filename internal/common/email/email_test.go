package email

import (
	"context"
	"strings"
	"testing"
)

func TestEmailTemplates(t *testing.T) {
	confirm := BuildConfirmationEmail("dev@scandrix.dev", "Acme Corp", "https://app.scandrix.dev/confirm?token=123")
	if !strings.Contains(confirm.HTMLBody, "Confirm your email address") {
		t.Fatalf("expected confirm title in HTML body")
	}
	if !strings.Contains(confirm.HTMLBody, "ScanDrix Inc.") {
		t.Fatalf("expected ScanDrix footer in HTML body")
	}
	if !strings.Contains(confirm.HTMLBody, "scandrix.dev") {
		t.Fatalf("expected scandrix.dev domain in HTML body")
	}

	forgot := BuildForgotPasswordEmail("dev@scandrix.dev", "https://app.scandrix.dev/reset?token=xyz")
	if !strings.Contains(forgot.HTMLBody, "Reset your password") {
		t.Fatalf("expected reset password title")
	}

	invite := BuildInviteEmail("invitee@scandrix.dev", "admin@scandrix.dev", "Acme Corp", "Core Engine", "https://app.scandrix.dev/invite/abc")
	if !strings.Contains(invite.HTMLBody, "You've been invited!") {
		t.Fatalf("expected invite title")
	}

	autoApproved := BuildReviewAutoApprovedEmail("dev@scandrix.dev", "scandrix/backend", 101, "Optimized connection pool", "https://github.com/scandrix/backend/pull/101")
	if !strings.Contains(autoApproved.HTMLBody, "Pull Request Auto-Approved ✨") {
		t.Fatalf("expected auto-approved title")
	}

	paymentFailed := BuildPaymentFailedEmail("dev@scandrix.dev", "$49.00", "Card declined", "March 25", "https://app.scandrix.dev/billing")
	if !strings.Contains(paymentFailed.HTMLBody, "Payment failed") || !strings.Contains(paymentFailed.HTMLBody, "$49.00") {
		t.Fatalf("expected payment failed body")
	}

	trialExpiring := BuildTrialExpiringEmail("dev@scandrix.dev", 3, "March 30", "https://app.scandrix.dev/upgrade")
	if !strings.Contains(trialExpiring.Subject, "3 days") || !strings.Contains(trialExpiring.HTMLBody, "Drixy") {
		t.Fatalf("expected trial expiring template with Drixy")
	}

	spendExceeded := BuildSpendLimitExceededEmail("dev@scandrix.dev", "$1,000", "$1,250")
	if !strings.Contains(spendExceeded.HTMLBody, "Monthly spend limit exceeded") {
		t.Fatalf("expected spend limit exceeded template")
	}

	byokErrors := BuildByokErrorsThresholdEmail("dev@scandrix.dev", "Anthropic", 25, "10:00 UTC", "11:00 UTC", "Rate limit 429")
	if !strings.Contains(byokErrors.HTMLBody, "BYOK LLM errors above threshold") {
		t.Fatalf("expected BYOK errors template")
	}

	memberRemoved := BuildMemberRemovedEmail("dev@scandrix.dev", "Alex", "Acme", "admin@scandrix.dev")
	if !strings.Contains(memberRemoved.HTMLBody, "You've been removed") {
		t.Fatalf("expected member removed template")
	}

	roleChanged := BuildOrgRoleChangedEmail("dev@scandrix.dev", "dev@scandrix.dev", "contributor", "repo_admin", "Acme", "admin@scandrix.dev")
	if !strings.Contains(roleChanged.HTMLBody, "Member role changed") {
		t.Fatalf("expected role changed template")
	}

	ideSynced := BuildIDERulesSyncedEmail("dev@scandrix.dev", "scandrix/backend", 5, "https://app.scandrix.dev/rules")
	if !strings.Contains(ideSynced.HTMLBody, "IDE rules synced") || !strings.Contains(ideSynced.HTMLBody, "Drixy Rules") {
		t.Fatalf("expected IDE rules synced template")
	}

	ideFailed := BuildIDERulesSyncFailedEmail("dev@scandrix.dev", "scandrix/backend", "Network timeout", "corr-12345")
	if !strings.Contains(ideFailed.HTMLBody, "IDE rule sync failed") {
		t.Fatalf("expected IDE sync failed template")
	}

	reviewSkipped := BuildReviewSkippedNoLicenseEmail("dev@scandrix.dev", "https://github.com/scandrix/backend/pull/5", "scandrix/backend", "admin@scandrix.dev", "coder")
	if !strings.Contains(reviewSkipped.HTMLBody, "Review skipped — license required") {
		t.Fatalf("expected review skipped template")
	}

	ruleBroken := BuildRuleFileReferencesInvalidEmail("dev@scandrix.dev", "scandrix/backend", 1, []RuleFileIssue{
		{RuleName: "No Global State", FilePath: "src/globals.ts", Reason: "File removed"},
	})
	if !strings.Contains(ruleBroken.HTMLBody, "Drixy rule file references are broken") {
		t.Fatalf("expected rule broken template")
	}

	orgReport := BuildOrgReportEmail("dev@scandrix.dev", "Acme Corp", "Weekly", 42, 0.85, 18.5, []OrgReportRanking{
		{Rank: 1, Repository: "scandrix/backend", Reviews: 25, ImplementationRate: 0.90},
	}, nil)
	if !strings.Contains(orgReport.HTMLBody, "Acme Corp Code Review Digest") {
		t.Fatalf("expected org report template")
	}

	repoReport := BuildRepoReportEmail("dev@scandrix.dev", "scandrix/backend", "Weekly", 25, 0.90, 12.0, nil)
	if !strings.Contains(repoReport.HTMLBody, "scandrix/backend Review Digest") {
		t.Fatalf("expected repo report template")
	}
}

func TestEmailServiceSandbox(t *testing.T) {
	svc := NewService(EmailConfig{
		ResendAPIKey: "", // Sandbox mode
	})

	msg := BuildConfirmationEmail("test@scandrix.dev", "TestOrg", "https://app.scandrix.dev/confirm")
	err := svc.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error in sandbox mode, got %v", err)
	}
}
