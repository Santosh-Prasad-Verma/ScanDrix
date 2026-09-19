package channels

import (
	"fmt"
	"html"
	"strings"

	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/templates"
)

// EmailTemplateContext provides ambient runtime configuration for building URLs in emails.
type EmailTemplateContext struct {
	WebURL string
}

// ResolvedEmail encapsulates the message elements produced by a registered email template builder.
type ResolvedEmail struct {
	From    string
	Subject string
	HTML    string
	ReplyTo string
}

// EmailTemplateRegistry stores and evaluates email template builders across all events.
type EmailTemplateRegistry struct {
	webURL string
}

// NewEmailTemplateRegistry initializes an email template registry.
func NewEmailTemplateRegistry(webURL string) *EmailTemplateRegistry {
	if webURL == "" {
		webURL = "https://app.scandrix.dev"
	}
	return &EmailTemplateRegistry{webURL: webURL}
}

// ResolveEmail formats the email subject, from address, and responsive HTML body for an event.
func (r *EmailTemplateRegistry) ResolveEmail(event catalog.Event, payload map[string]interface{}) *ResolvedEmail {
	fromDefault := "ScanDrix <notifications@scandrix.dev>"
	fromSecurity := "ScanDrix Security <security@scandrix.dev>"
	fromBilling := "ScanDrix Billing <billing@scandrix.dev>"
	replyTo := "support@scandrix.dev"

	switch event {
	case catalog.EventAuthEmailConfirmation:
		token := getString(payload, "token")
		org := getStringDefault(payload, "organizationName", "your workspace")
		ctaURL := fmt.Sprintf("%s/auth/confirm-email?token=%s", r.webURL, token)
		title := "Confirm your email address"
		bodyHTML := fmt.Sprintf(`
			<p>Welcome to ScanDrix! Please confirm your email address to activate your account and access <strong>%s</strong>.</p>
			<p>This verification link expires in 24 hours. If you did not create this account, you can safely disregard this message.</p>
		`, html.EscapeString(org))
		return &ResolvedEmail{
			From:    fromSecurity,
			Subject: "Confirm your email for ScanDrix",
			HTML:    templates.RenderBrandLayout("Verify your ScanDrix account", title, bodyHTML, "Confirm Email", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventAuthForgotPassword:
		token := getString(payload, "token")
		name := getStringDefault(payload, "name", "there")
		ctaURL := fmt.Sprintf("%s/forgot-password/reset?token=%s", r.webURL, token)
		title := "Reset your password"
		bodyHTML := fmt.Sprintf(`
			<p>Hello %s,</p>
			<p>We received a request to reset the password for your ScanDrix account. Click below to choose a new password.</p>
			<p style="color: #6b7280; font-size: 13px;">If you didn't request a password reset, you can safely ignore this email. Your password will not change.</p>
		`, html.EscapeString(name))
		return &ResolvedEmail{
			From:    fromSecurity,
			Subject: "Reset your ScanDrix password",
			HTML:    templates.RenderBrandLayout("Password reset request", title, bodyHTML, "Reset Password", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventTeamMemberInvited:
		inviter := getStringDefault(payload, "inviterEmail", "A teammate")
		inviteLink := getStringDefault(payload, "inviteLink", r.webURL+"/join")
		title := "You've been invited to join ScanDrix"
		bodyHTML := fmt.Sprintf(`
			<p><strong>%s</strong> has invited you to collaborate on automated code reviews with ScanDrix.</p>
			<p>Join your team to get automated AI code reviews, custom security rules, and real-time PR insights.</p>
		`, html.EscapeString(inviter))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: "You've been invited to join ScanDrix",
			HTML:    templates.RenderBrandLayout("Team collaboration invite", title, bodyHTML, "Accept Invitation", inviteLink),
			ReplyTo: replyTo,
		}

	case catalog.EventDrixyRulesGenerated:
		org := getStringDefault(payload, "organizationName", "your workspace")
		rules := getStrings(payload, "rules")
		ctaURL := r.webURL + "/library/drixy-rules"
		title := "New Drixy rules generated"
		var listItems strings.Builder
		for _, rule := range rules {
			listItems.WriteString(fmt.Sprintf("<li>%s</li>", html.EscapeString(rule)))
		}
		if len(rules) == 0 {
			listItems.WriteString("<li>New repository conventions learned from recent PR reviews.</li>")
		}
		bodyHTML := fmt.Sprintf(`
			<p>Drixy has analyzed your recent code reviews for <strong>%s</strong> and synthesized new custom review rules:</p>
			<ul style="margin: 16px 0; padding-left: 20px; line-height: 1.8;">
				%s
			</ul>
			<p>Review these suggestions and approve the rules you want active in your automated review pipeline.</p>
		`, html.EscapeString(org), listItems.String())
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: "New Drixy rules generated for " + org,
			HTML:    templates.RenderBrandLayout("New custom review rules", title, bodyHTML, "Review Rules", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventSSODomainVerification:
		domain := getString(payload, "domain")
		org := getStringDefault(payload, "organizationName", "your organization")
		token := getString(payload, "token")
		ctaURL := fmt.Sprintf("%s/organization/sso?verify=%s", r.webURL, token)
		title := "Verify your SSO domain"
		bodyHTML := fmt.Sprintf(`
			<p>To enable SAML Single Sign-On for <strong>%s</strong>, please complete verification for the domain <code>%s</code>.</p>
			<p>Click the button below to confirm domain ownership.</p>
		`, html.EscapeString(org), html.EscapeString(domain))
		return &ResolvedEmail{
			From:    fromSecurity,
			Subject: "Verify your SSO domain for ScanDrix: " + domain,
			HTML:    templates.RenderBrandLayout("SSO domain verification", title, bodyHTML, "Verify Domain", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventRepoReport:
		ctaURL := r.webURL + "/cockpit"
		title := "Your ScanDrix repository digest is ready"
		bodyHTML := `
			<p>Your weekly repository code quality and review digest is now available.</p>
			<p>Check out key metrics: PR review turnaround times, recurring rule infractions, and automated suggestion acceptance rates.</p>
		`
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: "Your ScanDrix Repository Digest",
			HTML:    templates.RenderBrandLayout("Weekly code quality metrics", title, bodyHTML, "View Repo Digest", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventOrgReport:
		ctaURL := r.webURL + "/cockpit"
		title := "Your ScanDrix organization recap is ready"
		bodyHTML := `
			<p>Here is your high-level overview of team velocity, security compliance, and review automated approvals this past week.</p>
		`
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: "Your ScanDrix Organization Recap",
			HTML:    templates.RenderBrandLayout("Organization code review report", title, bodyHTML, "Open Cockpit", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventReviewAutoApproved:
		repo := getStringDefault(payload, "repoName", "Repository")
		prURL := getString(payload, "prUrl")
		title := "Pull request auto-approved"
		bodyHTML := fmt.Sprintf(`
			<p>A pull request in <strong>%s</strong> passed all automated quality, security, and rule checks and was auto-approved by Drixy.</p>
		`, html.EscapeString(repo))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: fmt.Sprintf("[%s] PR Auto-Approved by Drixy", repo),
			HTML:    templates.RenderBrandLayout("PR auto-approved", title, bodyHTML, "View Pull Request", prURL),
			ReplyTo: replyTo,
		}

	case catalog.EventReviewFailed:
		repo := getStringDefault(payload, "repoName", "Repository")
		reason := getStringDefault(payload, "reason", "review runner failure")
		prURL := getString(payload, "prUrl")
		title := "Automated code review failed"
		bodyHTML := fmt.Sprintf(`
			<p>Drixy could not complete the review for a pull request in <strong>%s</strong>.</p>
			<div style="background-color: #fee2e2; border-left: 4px solid #ef4444; padding: 12px 16px; margin: 16px 0; border-radius: 4px; color: #991b1b;">
				<strong>Error:</strong> %s
			</div>
		`, html.EscapeString(repo), html.EscapeString(reason))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: fmt.Sprintf("[%s] Code review failed", repo),
			HTML:    templates.RenderBrandLayout("Code review error", title, bodyHTML, "Inspect Review", prURL),
			ReplyTo: replyTo,
		}

	case catalog.EventReviewSkippedNoLicense:
		repo := getStringDefault(payload, "repoName", "Repository")
		author := getStringDefault(payload, "authorUsername", "Contributor")
		ownerContact := getStringDefault(payload, "ownerContact", "your workspace administrator")
		ctaURL := r.webURL + "/settings/team"
		if pr := getString(payload, "prUrl"); pr != "" {
			ctaURL = pr
		}
		title := "Code review skipped — license required"
		bodyHTML := fmt.Sprintf(`
			<p>A pull request created by <strong>%s</strong> in <strong>%s</strong> was skipped because the author does not have an active seat license.</p>
			<p>Please contact %s to assign a developer license and enable reviews for this contributor.</p>
		`, html.EscapeString(author), html.EscapeString(repo), html.EscapeString(ownerContact))
		return &ResolvedEmail{
			From:    fromBilling,
			Subject: fmt.Sprintf("[%s] Review skipped (License required)", repo),
			HTML:    templates.RenderBrandLayout("Review license required", title, bodyHTML, "Manage Licenses", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventBillingPaymentFailed:
		amt := getFloat(payload, "amount")
		curr := getStringDefault(payload, "currency", "USD")
		reason := getStringDefault(payload, "failureReason", "declined")
		formatted := fmt.Sprintf("%s %.2f", curr, amt/100.0)
		ctaURL := getStringDefault(payload, "updatePaymentUrl", r.webURL+"/billing")
		title := "Payment processing failed"
		bodyHTML := fmt.Sprintf(`
			<p>We were unable to process the subscription payment of <strong>%s</strong> for your ScanDrix account.</p>
			<div style="background-color: #fef2f2; border: 1px solid #fecaca; padding: 12px 16px; margin: 16px 0; border-radius: 6px; color: #b91c1c;">
				<strong>Reason:</strong> %s
			</div>
			<p>Please update your billing method to avoid interruption to your automated code reviews.</p>
		`, html.EscapeString(formatted), html.EscapeString(reason))
		return &ResolvedEmail{
			From:    fromBilling,
			Subject: "Payment failed for your ScanDrix subscription",
			HTML:    templates.RenderBrandLayout("Payment failure notification", title, bodyHTML, "Update Payment Method", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventBillingTrialExpiring:
		days := getInt(payload, "daysRemaining")
		ctaURL := getStringDefault(payload, "upgradeUrl", r.webURL+"/billing/upgrade")
		rem := fmt.Sprintf("%d days", days)
		if days == 1 {
			rem = "tomorrow"
		}
		title := "Your ScanDrix trial expires " + rem
		bodyHTML := fmt.Sprintf(`
			<p>Your free trial of ScanDrix will expire in <strong>%s</strong>.</p>
			<p>Upgrade to a paid plan today to ensure continuous automated code reviews and keep your security policies active.</p>
		`, rem)
		return &ResolvedEmail{
			From:    fromBilling,
			Subject: "Your ScanDrix trial is expiring soon",
			HTML:    templates.RenderBrandLayout("Trial expiration warning", title, bodyHTML, "Upgrade Subscription", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventByokLlmErrorsThreshold:
		provider := getStringDefault(payload, "provider", "BYOK")
		count := getInt(payload, "errorCount")
		latest := getStringDefault(payload, "sampleError", "rate limit exceeded")
		ctaURL := r.webURL + "/settings/byok"
		title := "BYOK LLM errors exceeded threshold"
		bodyHTML := fmt.Sprintf(`
			<p>Your custom <strong>%s</strong> model provider has generated <strong>%d errors</strong> in the last 15 minutes.</p>
			<div style="background-color: #fef3c7; border-left: 4px solid #f59e0b; padding: 12px 16px; margin: 16px 0; border-radius: 4px; color: #92400e;">
				<strong>Recent error:</strong> %s
			</div>
			<p>Please check your provider credentials, rate limits, or model endpoint health.</p>
		`, html.EscapeString(provider), count, html.EscapeString(latest))
		return &ResolvedEmail{
			From:    fromSecurity,
			Subject: "Action Required: BYOK LLM Errors Exceeded Threshold",
			HTML:    templates.RenderBrandLayout("BYOK error threshold alert", title, bodyHTML, "Manage BYOK Settings", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventSpendLimitThresholdReached:
		pct := getInt(payload, "percentage")
		spent := getFloat(payload, "spentUsd")
		limit := getFloat(payload, "monthlyLimitUsd")
		ctaURL := r.webURL + "/settings/spend-limits"
		title := fmt.Sprintf("BYOK spend reached %d%% of monthly limit", pct)
		bodyHTML := fmt.Sprintf(`
			<p>Your BYOK LLM usage this month has reached <strong>$%.2f</strong> of your <strong>$%.2f</strong> limit (<strong>%d%%</strong>).</p>
			<p>This is an informational warning — code reviews will continue running without interruption.</p>
		`, spent, limit, pct)
		return &ResolvedEmail{
			From:    fromBilling,
			Subject: fmt.Sprintf("Notice: BYOK monthly spend has reached %d%% of limit", pct),
			HTML:    templates.RenderBrandLayout("Spend limit threshold", title, bodyHTML, "Review Spend Limits", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventSpendLimitExceededFinal:
		spent := getFloat(payload, "spentUsd")
		limit := getFloat(payload, "monthlyLimitUsd")
		ctaURL := r.webURL + "/settings/spend-limits"
		title := "Monthly BYOK spend limit exceeded"
		bodyHTML := fmt.Sprintf(`
			<p>Your BYOK spend has passed your monthly budget limit: <strong>$%.2f</strong> spent against <strong>$%.2f</strong> limit.</p>
			<p>To avoid unexpected cloud costs, ensure you have hard caps configured with your model provider.</p>
		`, spent, limit)
		return &ResolvedEmail{
			From:    fromBilling,
			Subject: "Alert: BYOK monthly spend limit exceeded",
			HTML:    templates.RenderBrandLayout("Monthly spend limit exceeded", title, bodyHTML, "Manage Spend Caps", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventRuleFileReferencesInvalid:
		count := getInt(payload, "invalidCount")
		repo := getStringDefault(payload, "repoName", "your repository")
		ctaURL := r.webURL + "/library/drixy-rules"
		title := "Drixy rule file references are invalid"
		bodyHTML := fmt.Sprintf(`
			<p>ScanDrix detected that <strong>%d custom review rules</strong> in <strong>%s</strong> reference file patterns that no longer match.</p>
			<p>These rules are temporarily bypassed during automated review runs until updated.</p>
		`, count, html.EscapeString(repo))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: fmt.Sprintf("[%s] Custom rule file references are invalid", repo),
			HTML:    templates.RenderBrandLayout("Rule file references invalid", title, bodyHTML, "Update Rules", ctaURL),
			ReplyTo: replyTo,
		}

	case catalog.EventOrgMemberRemoved:
		org := getStringDefault(payload, "organizationName", "the organization")
		title := "Member removed from " + org
		bodyHTML := fmt.Sprintf("<p>An account was removed from <strong>%s</strong>.</p>", html.EscapeString(org))
		return &ResolvedEmail{
			From:    fromSecurity,
			Subject: "Member removed from " + org,
			HTML:    templates.RenderBrandLayout("Team membership update", title, bodyHTML, "View Organization", r.webURL+"/settings/team"),
			ReplyTo: replyTo,
		}

	case catalog.EventOrgRoleChanged:
		org := getStringDefault(payload, "organizationName", "the organization")
		title := "Role changed in " + org
		bodyHTML := fmt.Sprintf("<p>A team member's role permissions were updated in <strong>%s</strong>.</p>", html.EscapeString(org))
		return &ResolvedEmail{
			From:    fromSecurity,
			Subject: "Role changed in " + org,
			HTML:    templates.RenderBrandLayout("Team role update", title, bodyHTML, "View Team", r.webURL+"/settings/team"),
			ReplyTo: replyTo,
		}

	case catalog.EventIDERulesSynced:
		repo := getStringDefault(payload, "repoName", "your repository")
		count := getInt(payload, "rulesCount")
		title := "IDE rules synced"
		bodyHTML := fmt.Sprintf("<p>%d rules synced successfully from <strong>%s</strong>.</p>", count, html.EscapeString(repo))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: fmt.Sprintf("[%s] IDE rules synced", repo),
			HTML:    templates.RenderBrandLayout("IDE rules synced", title, bodyHTML, "View Rules", r.webURL+"/library/drixy-rules"),
			ReplyTo: replyTo,
		}

	case catalog.EventIDERulesSyncFailed:
		repo := getStringDefault(payload, "repoName", "your repository")
		reason := getStringDefault(payload, "reason", "sync timeout")
		title := "IDE rule sync failed"
		bodyHTML := fmt.Sprintf("<p>Drixy could not sync rules from <strong>%s</strong>: %s.</p>", html.EscapeString(repo), html.EscapeString(reason))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: fmt.Sprintf("[%s] IDE rule sync failed", repo),
			HTML:    templates.RenderBrandLayout("Rule sync failed", title, bodyHTML, "Inspect Logs", r.webURL+"/settings/rules"),
			ReplyTo: replyTo,
		}

	default:
		title := string(event)
		if def, ok := catalog.EventDefaultsMap[event]; ok {
			title = def.Label
		}
		bodyHTML := fmt.Sprintf("<p>New notification event: %s</p>", string(event))
		return &ResolvedEmail{
			From:    fromDefault,
			Subject: "ScanDrix Notification: " + title,
			HTML:    templates.RenderBrandLayout("System notification", title, bodyHTML, "Open Dashboard", r.webURL),
			ReplyTo: replyTo,
		}
	}
}

func getStrings(m map[string]interface{}, key string) []string {
	var result []string
	if raw, ok := m[key].([]interface{}); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	} else if raw, ok := m[key].([]string); ok {
		return raw
	}
	return result
}
