// Package email provides transactional templates for accounts, security, reviews, and billing.
package email

import (
	"fmt"
	"strings"
)

// EmailMessage contains the final rendered subject, plain text, and HTML body.
type EmailMessage struct {
	From      EmailFrom `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	HTMLBody  string    `json:"htmlBody"`
	TextBody  string    `json:"textBody"`
	ReplyTo   string    `json:"replyTo,omitempty"`
}

// BuildConfirmationEmail renders the email confirmation template.
func BuildConfirmationEmail(email, orgName, confirmLink string) EmailMessage {
	subject := fmt.Sprintf("Confirm your email for %s on ScanDrix", orgName)
	preview := "Confirm your email to complete registration."
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">Confirm your email address</h2>
<p>You have been invited to join <strong>%s</strong> on ScanDrix, the AI code review platform.</p>
<p>Please click the button below to confirm your email and activate your account:</p>
<div style="text-align:center;">
  <a href="%s" class="btn">Confirm Email Address</a>
</div>
<p style="font-size:13px;color:#6B7280;">If you didn't create an account with ScanDrix, you can safely ignore this email.</p>
<p style="font-size:12px;color:#9CA3AF;word-break:break-all;">Link: %s</p>
`, orgName, confirmLink, confirmLink)

	textBody := fmt.Sprintf("Confirm your email address for %s on ScanDrix:\n\n%s\n\nIf you didn't create this account, please ignore this email.", orgName, confirmLink)

	return EmailMessage{
		From:     FromNoReply,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildForgotPasswordEmail renders the password reset template.
func BuildForgotPasswordEmail(email, resetLink string) EmailMessage {
	subject := "Reset your ScanDrix password"
	preview := "Reset instructions for your ScanDrix account."
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">Reset your password</h2>
<p>We received a request to reset the password for your ScanDrix account (<strong>%s</strong>).</p>
<p>Click the button below to set a new password:</p>
<div style="text-align:center;">
  <a href="%s" class="btn">Reset Password</a>
</div>
<p style="font-size:13px;color:#6B7280;">This password reset link is valid for 1 hour. If you did not request a password reset, no action is required.</p>
`, email, resetLink)

	textBody := fmt.Sprintf("Reset your ScanDrix password:\n\n%s\n\nThis link expires in 1 hour. If you did not request this, ignore this email.", resetLink)

	return EmailMessage{
		From:     FromSecurity,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildInviteEmail renders the team invitation email template.
func BuildInviteEmail(email, inviterEmail, orgName, teamName, inviteLink string) EmailMessage {
	subject := fmt.Sprintf("%s invited you to join %s on ScanDrix", inviterEmail, orgName)
	preview := fmt.Sprintf("Join %s on ScanDrix", orgName)
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">You've been invited!</h2>
<p><strong>%s</strong> has invited you to collaborate on the <strong>%s</strong> team within <strong>%s</strong> on ScanDrix.</p>
<div style="text-align:center;">
  <a href="%s" class="btn">Accept Invitation</a>
</div>
<p style="font-size:13px;color:#6B7280;">Get automated code reviews, architectural guardrails, and real-time pull request analysis.</p>
`, inviterEmail, teamName, orgName, inviteLink)

	textBody := fmt.Sprintf("%s invited you to join %s on ScanDrix.\n\nAccept your invitation here:\n%s", inviterEmail, orgName, inviteLink)

	return EmailMessage{
		From:     FromNoReply,
		To:       email,
		Subject:  subject,
		ReplyTo:  inviterEmail,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildDomainVerificationEmail renders the SSO domain verification email template.
func BuildDomainVerificationEmail(email, orgName, domain, verifyLink string) EmailMessage {
	subject := fmt.Sprintf("Verify domain %s for %s", domain, orgName)
	preview := fmt.Sprintf("Verify domain %s for Single Sign-On.", domain)
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">Verify Domain for SSO</h2>
<p>A request was made to verify the domain <strong>%s</strong> for organization <strong>%s</strong> on ScanDrix.</p>
<div style="text-align:center;">
  <a href="%s" class="btn">Verify Domain</a>
</div>
`, domain, orgName, verifyLink)

	textBody := fmt.Sprintf("Verify domain %s for %s on ScanDrix:\n%s", domain, orgName, verifyLink)

	return EmailMessage{
		From:     FromSecurity,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildDrixyRulesEmail renders notification when repository rules are synced.
func BuildDrixyRulesEmail(email, orgName string, rules []string, rulesLink string) EmailMessage {
	subject := fmt.Sprintf("New code review rules updated for %s", orgName)
	preview := fmt.Sprintf("%d code review rules updated.", len(rules))

	var listHTML strings.Builder
	for _, r := range rules {
		listHTML.WriteString(fmt.Sprintf("<li style='margin-bottom:6px;'>%s</li>", r))
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">Review Rules Updated</h2>
<p>The following custom review rules are now active for <strong>%s</strong>:</p>
<ul style="background:#F9FAFB;padding:16px 24px;border-radius:6px;border:1px solid #E5E7EB;">
%s
</ul>
<div style="text-align:center;">
  <a href="%s" class="btn">View Rules Library</a>
</div>
`, orgName, listHTML.String(), rulesLink)

	textBody := fmt.Sprintf("Review rules updated for %s. View rules at %s", orgName, rulesLink)

	return EmailMessage{
		From:     FromAlerts,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildReviewAutoApprovedEmail renders notification when a PR is automatically approved by Drixy.
func BuildReviewAutoApprovedEmail(email, repoName string, prNumber int, prTitle, prURL string) EmailMessage {
	subject := fmt.Sprintf("[Auto-Approved] PR #%d in %s passed review", prNumber, repoName)
	preview := fmt.Sprintf("PR #%d passed all automated checks.", prNumber)
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#10B981;">Pull Request Auto-Approved ✨</h2>
<p>Pull Request <strong>#%d: %s</strong> in <strong>%s</strong> has passed all automated quality gates and rule validations with zero critical issues.</p>
<div style="text-align:center;">
  <a href="%s" class="btn">View Pull Request</a>
</div>
`, prNumber, prTitle, repoName, prURL)

	textBody := fmt.Sprintf("PR #%d (%s) in %s was auto-approved.\n%s", prNumber, prTitle, repoName, prURL)

	return EmailMessage{
		From:     FromAlerts,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildReviewFailedEmail renders notification when a code review fails due to fatal errors.
func BuildReviewFailedEmail(email, repoName string, prNumber int, reason, prURL string) EmailMessage {
	subject := fmt.Sprintf("[Action Required] Review failed for PR #%d in %s", prNumber, repoName)
	preview := "Review execution failed."
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">Code Review Failed</h2>
<p>An error occurred while executing automated review on PR <strong>#%d</strong> in <strong>%s</strong>:</p>
<div style="background:#FEE2E2;color:#991B1B;padding:12px 16px;border-radius:6px;margin:16px 0;font-family:monospace;font-size:13px;">
%s
</div>
<div style="text-align:center;">
  <a href="%s" class="btn">Check Pull Request</a>
</div>
`, prNumber, repoName, reason, prURL)

	textBody := fmt.Sprintf("Code review failed for PR #%d in %s: %s\n%s", prNumber, repoName, reason, prURL)

	return EmailMessage{
		From:     FromAlerts,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildSpendLimitThresholdEmail renders alert when spend reaches 80% or 90%.
func BuildSpendLimitThresholdEmail(email, orgName string, currentSpend, limit float64, threshold int) EmailMessage {
	subject := fmt.Sprintf("[Alert] %s reached %d%% of monthly spend limit", orgName, threshold)
	preview := fmt.Sprintf("Current spend is $%.2f of $%.2f limit.", currentSpend, limit)
	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#F59E0B;">Spend Limit Alert: %d%% Reached</h2>
<p>Organization <strong>%s</strong> has used <strong>$%.2f</strong> of its <strong>$%.2f</strong> monthly budget.</p>
<p>To ensure uninterrupted code reviews across your repositories, consider adjusting your budget threshold.</p>
<div style="text-align:center;">
  <a href="https://app.scandrix.dev/organization/billing" class="btn">Manage Billing</a>
</div>
`, threshold, orgName, currentSpend, limit)

	textBody := fmt.Sprintf("Alert: %s reached %d%% of spend limit ($%.2f / $%.2f). Manage billing at https://app.scandrix.dev/organization/billing", orgName, threshold, currentSpend, limit)

	return EmailMessage{
		From:     FromBilling,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildPaymentFailedEmail renders notification when a subscription payment attempt fails.
func BuildPaymentFailedEmail(email, formattedAmount, failureReason, nextRetryAtLabel, updatePaymentURL string) EmailMessage {
	subject := "Payment failed — action required"
	preview := fmt.Sprintf("Payment failed: %s. Update payment method to keep your subscription active.", formattedAmount)

	var retryMsg string
	if nextRetryAtLabel != "" {
		retryMsg = fmt.Sprintf("<p>We'll retry automatically on <strong>%s</strong>. To avoid an interruption to your service, update your payment method now.</p>", nextRetryAtLabel)
	} else {
		retryMsg = "<p>Update your payment method to avoid interrupting your subscription.</p>"
	}

	btnHTML := ""
	if updatePaymentURL != "" {
		btnHTML = fmt.Sprintf(`<div style="text-align:center;margin:24px 0;"><a href="%s" class="btn">Update payment method</a></div>`, updatePaymentURL)
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">Payment failed</h2>
<p>We could not process your payment of <strong>%s</strong>.</p>
<p><strong>Reason:</strong> %s</p>
%s
%s
`, formattedAmount, failureReason, retryMsg, btnHTML)

	textBody := fmt.Sprintf("Payment failed: %s. Reason: %s.\nUpdate your payment method at %s", formattedAmount, failureReason, updatePaymentURL)

	return EmailMessage{
		From:     FromBilling,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildTrialExpiringEmail renders reminder when a trial period is ending soon.
func BuildTrialExpiringEmail(email string, daysRemaining int, trialEndsAtLabel, upgradeURL string) EmailMessage {
	remaining := fmt.Sprintf("in %d days", daysRemaining)
	subject := fmt.Sprintf("Your ScanDrix trial ends in %d days", daysRemaining)
	if daysRemaining == 1 {
		remaining = "tomorrow"
		subject = "Your ScanDrix trial ends tomorrow"
	}

	preview := fmt.Sprintf("Your ScanDrix trial ends %s. Upgrade to keep things running.", remaining)

	btnHTML := ""
	if upgradeURL != "" {
		btnHTML = fmt.Sprintf(`<div style="text-align:center;margin:24px 0;"><a href="%s" class="btn">Upgrade plan</a></div>`, upgradeURL)
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">Your trial ends %s</h2>
<p>Your ScanDrix trial ends on <strong>%s</strong>. To keep Drixy reviewing your pull requests without interruption, upgrade your organization before then.</p>
%s
<p style="font-size:13px;color:#6B7280;">If you decide not to upgrade, your organization will move to the free tier with limited review capacity.</p>
`, remaining, trialEndsAtLabel, btnHTML)

	textBody := fmt.Sprintf("Your ScanDrix trial ends %s on %s. Upgrade at %s to keep Drixy reviewing without interruption.", remaining, trialEndsAtLabel, upgradeURL)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildSpendLimitExceededEmail renders alert when monthly BYOK spend limit is exceeded.
func BuildSpendLimitExceededEmail(email, limitLabel, spentLabel string) EmailMessage {
	subject := "BYOK monthly spend limit exceeded"
	preview := fmt.Sprintf("Your BYOK spend (%s) has passed your %s monthly limit.", spentLabel, limitLabel)

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">Monthly spend limit exceeded</h2>
<p>Your BYOK model spend this month (<strong>%s</strong>) has passed your <strong>%s</strong> limit.</p>
<p><strong>This is the last alert you'll get this month.</strong> Code reviews continue to run — to stop spend, set a hard limit in your model provider's billing dashboard.</p>
<p style="font-size:13px;color:#6B7280;">Your spend total resets at the start of next month.</p>
`, spentLabel, limitLabel)

	textBody := fmt.Sprintf("Your BYOK model spend this month (%s) has passed your %s limit. Manage limits in your provider dashboard.", spentLabel, limitLabel)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildByokErrorsThresholdEmail renders alert when a custom LLM provider errors exceed the threshold.
func BuildByokErrorsThresholdEmail(email, provider string, errorCount int, windowStartLabel, windowEndLabel, sampleError string) EmailMessage {
	subject := fmt.Sprintf("BYOK LLM errors — %s above threshold", provider)
	preview := fmt.Sprintf("Your BYOK %s model failed %d times in the recent window.", provider, errorCount)

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">BYOK LLM errors above threshold</h2>
<p>Your BYOK <strong>%s</strong> model returned <strong>%d</strong> errors between %s and %s. Code reviews using this configuration may be delayed or skipped while the outage continues.</p>
<p><strong>Latest error:</strong> <code>%s</code></p>
<p style="font-size:13px;color:#6B7280;">You will not receive another notification about this until a cooldown period elapses, even if errors continue. Check your provider dashboard or rotate keys if needed.</p>
`, provider, errorCount, windowStartLabel, windowEndLabel, sampleError)

	textBody := fmt.Sprintf("BYOK %s model returned %d errors between %s and %s. Latest error: %s", provider, errorCount, windowStartLabel, windowEndLabel, sampleError)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildMemberRemovedEmail renders notification when a user is removed from an organization.
func BuildMemberRemovedEmail(email, removedUserName, organizationName, removedBy string) EmailMessage {
	subject := fmt.Sprintf("You've been removed from %s", organizationName)
	preview := fmt.Sprintf("You've been removed from %s on ScanDrix", organizationName)

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">You've been removed</h2>
<p>Hi %s,</p>
<p>You have been removed from <strong>%s</strong> on ScanDrix by %s. You no longer have access to that organization's projects or settings.</p>
<p style="font-size:13px;color:#6B7280;">If you think this was a mistake, please contact your administrator directly.</p>
`, removedUserName, organizationName, removedBy)

	textBody := fmt.Sprintf("Hi %s,\nYou have been removed from %s on ScanDrix by %s.", removedUserName, organizationName, removedBy)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildOrgRoleChangedEmail renders notification when a member's role changes.
func BuildOrgRoleChangedEmail(email, affectedUserEmail, previousRole, newRole, organizationName, changedBy string) EmailMessage {
	subject := fmt.Sprintf("Role changed for %s in %s", affectedUserEmail, organizationName)
	preview := fmt.Sprintf("%s's role in %s changed to %s", affectedUserEmail, organizationName, newRole)

	changedByMsg := ""
	if changedBy != "" {
		changedByMsg = fmt.Sprintf("<p style='font-size:13px;color:#6B7280;'>Changed by %s.</p>", changedBy)
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">Member role changed</h2>
<p><strong>%s</strong>'s role in <strong>%s</strong> changed from <strong>%s</strong> to <strong>%s</strong>.</p>
%s
<p style="font-size:13px;color:#6B7280;">You're receiving this because you're an owner of %s. If this wasn't expected, review your organization's members.</p>
`, affectedUserEmail, organizationName, previousRole, newRole, changedByMsg, organizationName)

	textBody := fmt.Sprintf("%s's role in %s changed from %s to %s.", affectedUserEmail, organizationName, previousRole, newRole)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildIDERulesSyncedEmail renders notification when IDE rules are synchronized into Drixy Rules.
func BuildIDERulesSyncedEmail(email, repoName string, rulesCount int, rulesLink string) EmailMessage {
	ruleWord := "rules"
	if rulesCount == 1 {
		ruleWord = "rule"
	}
	subject := fmt.Sprintf("IDE rules synced — %s", repoName)
	preview := fmt.Sprintf("Synced %d %s from %s", rulesCount, ruleWord, repoName)

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">IDE rules synced</h2>
<p>Drixy synced <strong>%d</strong> %s from <strong>%s</strong> into your Drixy Rules.</p>
<div style="text-align:center;margin:24px 0;">
  <a href="%s" class="btn">View Drixy Rules</a>
</div>
<p style="font-size:13px;color:#6B7280;">Manage IDE-sync notifications in your notification settings.</p>
`, rulesCount, ruleWord, repoName, rulesLink)

	textBody := fmt.Sprintf("Drixy synced %d %s from %s into your Drixy Rules. View them at %s", rulesCount, ruleWord, repoName, rulesLink)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildIDERulesSyncFailedEmail renders alert when IDE rules sync fails.
func BuildIDERulesSyncFailedEmail(email, repoName, reason, correlationID string) EmailMessage {
	subject := fmt.Sprintf("IDE rule sync failed — %s", repoName)
	preview := fmt.Sprintf("Drixy could not sync IDE rules in %s", repoName)

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">IDE rule sync failed</h2>
<p>Drixy could not finish syncing IDE rules from <strong>%s</strong>.</p>
<p><strong>Reason:</strong> %s</p>
<p style="font-size:13px;color:#6B7280;">You can retry the sync from your repository settings. If this persists, share this reference with support: <code>%s</code>.</p>
`, repoName, reason, correlationID)

	textBody := fmt.Sprintf("Drixy could not finish syncing IDE rules from %s. Reason: %s. Reference: %s", repoName, reason, correlationID)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildReviewSkippedNoLicenseEmail renders notification when a PR review is skipped due to inactive license.
func BuildReviewSkippedNoLicenseEmail(email, prURL, repoName, ownerContact, authorUsername string) EmailMessage {
	if authorUsername == "" {
		authorUsername = "an unknown user"
	}
	subject := fmt.Sprintf("Review skipped — license required (%s)", repoName)
	preview := fmt.Sprintf("A pull request in %s wasn't reviewed — no active license", repoName)

	contactMsg := "Contact your organization admin to enable reviews."
	if ownerContact != "" {
		contactMsg = fmt.Sprintf("Contact <strong>%s</strong> to enable reviews.", ownerContact)
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">Review skipped — license required</h2>
<p>A pull request by <strong>%s</strong> in <strong>%s</strong> was not reviewed because this user doesn't have an active license.</p>
<p>%s</p>
<div style="text-align:center;margin:24px 0;">
  <a href="%s" class="btn">Open pull request</a>
</div>
<p style="font-size:13px;color:#6B7280;">Reviews resume automatically once an active license is in place.</p>
`, authorUsername, repoName, contactMsg, prURL)

	textBody := fmt.Sprintf("A pull request by %s in %s was not reviewed because this user doesn't have an active license.\n%s\n%s", authorUsername, repoName, contactMsg, prURL)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// RuleFileIssue describes a rule with a broken file target.
type RuleFileIssue struct {
	RuleName string `json:"ruleName"`
	FilePath string `json:"filePath"`
	Reason   string `json:"reason"`
}

// BuildRuleFileReferencesInvalidEmail renders notification when Drixy rules point to invalid/missing files.
func BuildRuleFileReferencesInvalidEmail(email, repoName string, invalidCount int, issues []RuleFileIssue) EmailMessage {
	ruleWord := "rules have"
	if invalidCount == 1 {
		ruleWord = "rule has"
	}
	subject := fmt.Sprintf("%d Drixy %s broken file references — %s", invalidCount, ruleWord, repoName)
	preview := fmt.Sprintf("%d %s a broken file reference in %s.", invalidCount, ruleWord, repoName)

	var issuesHTML strings.Builder
	displayed := issues
	if len(displayed) > 10 {
		displayed = displayed[:10]
	}
	for _, issue := range displayed {
		issuesHTML.WriteString(fmt.Sprintf("<p style='margin:8px 0;'><strong>%s</strong> &rarr; <code>%s</code><br/><span style='font-size:12px;color:#6B7280;'>%s</span></p>", issue.RuleName, issue.FilePath, issue.Reason))
	}
	if len(issues) > len(displayed) {
		issuesHTML.WriteString(fmt.Sprintf("<p style='font-size:12px;color:#6B7280;'>...and %d more affected rules.</p>", len(issues)-len(displayed)))
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#EF4444;">Drixy rule file references are broken</h2>
<p><strong>%d</strong> rules in <strong>%s</strong> reference a file that no longer exists or no longer matches. Affected rules are skipped during code review until you fix or remove the reference.</p>
<div style="background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;margin:16px 0;">
%s
</div>
`, invalidCount, repoName, issuesHTML.String())

	textBody := fmt.Sprintf("%d Drixy rules in %s reference broken files. Affected rules are skipped until fixed.", invalidCount, repoName)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// OrgReportRanking represents a repository rank in the periodic digest.
type OrgReportRanking struct {
	Rank               int     `json:"rank"`
	Repository         string  `json:"repository"`
	Reviews            int     `json:"reviews"`
	ImplementationRate float64 `json:"implementationRate"`
}

// OrgReportRule represents rule triggers in the periodic digest.
type OrgReportRule struct {
	Title    string `json:"title"`
	Triggers int    `json:"triggers"`
}

// BuildOrgReportEmail renders the weekly or monthly executive digest for an organization.
func BuildOrgReportEmail(
	email, orgName, periodLabel string,
	totalReviews int,
	implementationRate float64,
	hoursSaved float64,
	rankings []OrgReportRanking,
	rules []OrgReportRule,
) EmailMessage {
	subject := fmt.Sprintf("%s Code Review Digest — %s", orgName, periodLabel)
	preview := fmt.Sprintf("%d pull requests reviewed, %.1f hours saved for %s.", totalReviews, hoursSaved, orgName)

	var rankingRows strings.Builder
	for _, r := range rankings {
		rankingRows.WriteString(fmt.Sprintf(`
<tr>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;">#%d</td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;"><strong>%s</strong></td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;text-align:right;">%d</td>
  <td style="padding:8px 12px;border-bottom:1px solid #E5E7EB;text-align:right;">%.0f%%</td>
</tr>`, r.Rank, r.Repository, r.Reviews, r.ImplementationRate*100))
	}

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">%s Code Review Digest</h2>
<p style="color:#6B7280;margin-top:-8px;">%s</p>

<div style="display:flex;gap:16px;margin:24px 0;">
  <div style="flex:1;background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;text-align:center;">
    <div style="font-size:24px;font-weight:700;color:#111827;">%d</div>
    <div style="font-size:12px;color:#6B7280;text-transform:uppercase;">Reviews</div>
  </div>
  <div style="flex:1;background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;text-align:center;">
    <div style="font-size:24px;font-weight:700;color:#10B981;">%.0f%%</div>
    <div style="font-size:12px;color:#6B7280;text-transform:uppercase;">Resolution Rate</div>
  </div>
  <div style="flex:1;background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;text-align:center;">
    <div style="font-size:24px;font-weight:700;color:#6366F1;">%.1fh</div>
    <div style="font-size:12px;color:#6B7280;text-transform:uppercase;">Hours Saved</div>
  </div>
</div>

<h3 style="font-size:16px;margin-top:24px;color:#111827;">Top Repositories</h3>
<table style="width:100%%;border-collapse:collapse;font-size:13px;">
  <thead>
    <tr style="background:#F3F4F6;color:#374151;text-align:left;">
      <th style="padding:8px 12px;">#</th>
      <th style="padding:8px 12px;">Repository</th>
      <th style="padding:8px 12px;text-align:right;">Reviews</th>
      <th style="padding:8px 12px;text-align:right;">Resolved</th>
    </tr>
  </thead>
  <tbody>
    %s
  </tbody>
</table>
<div style="text-align:center;margin:24px 0;">
  <a href="https://app.scandrix.dev/reports" class="btn">View Complete Analytics</a>
</div>
`, orgName, periodLabel, totalReviews, implementationRate*100, hoursSaved, rankingRows.String())

	textBody := fmt.Sprintf("%s Digest (%s): %d reviews, %.0f%% resolved, %.1f hours saved.\nhttps://app.scandrix.dev/reports", orgName, periodLabel, totalReviews, implementationRate*100, hoursSaved)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}

// BuildRepoReportEmail renders the periodic digest for an individual repository.
func BuildRepoReportEmail(
	email, repoName, periodLabel string,
	totalReviews int,
	implementationRate float64,
	hoursSaved float64,
	rules []OrgReportRule,
) EmailMessage {
	subject := fmt.Sprintf("%s Code Review Digest — %s", repoName, periodLabel)
	preview := fmt.Sprintf("%d pull requests reviewed in %s.", totalReviews, repoName)

	bodyHTML := fmt.Sprintf(`
<h2 style="margin-top:0;font-size:20px;color:#111827;">%s Review Digest</h2>
<p style="color:#6B7280;margin-top:-8px;">%s</p>

<div style="display:flex;gap:16px;margin:24px 0;">
  <div style="flex:1;background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;text-align:center;">
    <div style="font-size:24px;font-weight:700;color:#111827;">%d</div>
    <div style="font-size:12px;color:#6B7280;text-transform:uppercase;">Reviews</div>
  </div>
  <div style="flex:1;background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;text-align:center;">
    <div style="font-size:24px;font-weight:700;color:#10B981;">%.0f%%</div>
    <div style="font-size:12px;color:#6B7280;text-transform:uppercase;">Resolution Rate</div>
  </div>
  <div style="flex:1;background:#F9FAFB;padding:16px;border-radius:6px;border:1px solid #E5E7EB;text-align:center;">
    <div style="font-size:24px;font-weight:700;color:#6366F1;">%.1fh</div>
    <div style="font-size:12px;color:#6B7280;text-transform:uppercase;">Hours Saved</div>
  </div>
</div>
<div style="text-align:center;margin:24px 0;">
  <a href="https://app.scandrix.dev/reports" class="btn">View Repository Analytics</a>
</div>
`, repoName, periodLabel, totalReviews, implementationRate*100, hoursSaved)

	textBody := fmt.Sprintf("%s Digest (%s): %d reviews, %.0f%% resolved, %.1f hours saved.\nhttps://app.scandrix.dev/reports", repoName, periodLabel, totalReviews, implementationRate*100, hoursSaved)

	return EmailMessage{
		From:     FromNotifications,
		To:       email,
		Subject:  subject,
		HTMLBody: RenderBrandLayout(preview, bodyHTML),
		TextBody: textBody,
	}
}
