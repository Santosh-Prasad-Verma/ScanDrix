package channels

import (
	"fmt"

	"github.com/scandrix/backend/internal/notifications/application"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
)

// InAppTemplateRegistry manages template builders for the in-app notification drawer.
type InAppTemplateRegistry struct{}

// NewInAppTemplateRegistry creates a new in-app template registry.
func NewInAppTemplateRegistry() *InAppTemplateRegistry {
	return &InAppTemplateRegistry{}
}

// ResolveInAppTemplate constructs user-facing title, body, and CTA URL for a given event.
func (r *InAppTemplateRegistry) ResolveInAppTemplate(event catalog.Event, payload map[string]interface{}) application.InAppTemplateResult {
	ctaURL := getString(payload, "ctaUrl")

	switch event {
	case catalog.EventAuthEmailConfirmation:
		org := getStringDefault(payload, "organizationName", "your organization")
		return application.InAppTemplateResult{
			Title:  "Confirm your email",
			Body:   fmt.Sprintf("Confirm your email for %s.", org),
			CtaURL: ctaURL,
		}

	case catalog.EventAuthForgotPassword:
		return application.InAppTemplateResult{
			Title:  "Password reset",
			Body:   "A password reset was requested for your account.",
			CtaURL: ctaURL,
		}

	case catalog.EventTeamMemberInvited:
		return application.InAppTemplateResult{
			Title:  "Team invitation",
			Body:   "You've been invited to join a team on ScanDrix.",
			CtaURL: ctaURL,
		}

	case catalog.EventDrixyRulesGenerated:
		org := getStringDefault(payload, "organizationName", "your organization")
		return application.InAppTemplateResult{
			Title:  "Drixy rules generated",
			Body:   fmt.Sprintf("New Drixy rules have been generated for %s.", org),
			CtaURL: "/library/drixy-rules",
		}

	case catalog.EventSSODomainVerification:
		domain := getString(payload, "domain")
		return application.InAppTemplateResult{
			Title:  "Verify your SSO domain",
			Body:   fmt.Sprintf("Verify your SSO domain: %s", domain),
			CtaURL: "/organization/sso",
		}

	case catalog.EventOrgReport:
		return application.InAppTemplateResult{
			Title:  "Your ScanDrix report is ready",
			Body:   "Your organization review report is ready.",
			CtaURL: "/cockpit",
		}

	case catalog.EventRepoReport:
		return application.InAppTemplateResult{
			Title:  "Your ScanDrix repo digest is ready",
			Body:   "Your per-repo review digest is ready.",
			CtaURL: "/cockpit",
		}

	case catalog.EventReviewAutoApproved:
		repo := getStringDefault(payload, "repoName", "A pull request")
		pr := getString(payload, "prUrl")
		return application.InAppTemplateResult{
			Title:  "Pull request auto-approved",
			Body:   fmt.Sprintf("%s was auto-approved by Drixy.", repo),
			CtaURL: pr,
		}

	case catalog.EventReviewFailed:
		repo := getStringDefault(payload, "repoName", "a pull request")
		reason := getStringDefault(payload, "reason", "internal timeout")
		pr := getString(payload, "prUrl")
		return application.InAppTemplateResult{
			Title:  "Code review failed",
			Body:   fmt.Sprintf("Drixy could not review %s: %s.", repo, reason),
			CtaURL: pr,
		}

	case catalog.EventReviewSkippedNoLicense:
		author := getStringDefault(payload, "authorUsername", "developer")
		repo := getStringDefault(payload, "repoName", "a repository")
		owner := getStringDefault(payload, "ownerContact", "your admin")
		pr := getString(payload, "prUrl")
		return application.InAppTemplateResult{
			Title:  "Review skipped — license required",
			Body:   fmt.Sprintf("PR by %s in %s was not reviewed — this user doesn't have an active license. Contact %s to enable reviews.", author, repo, owner),
			CtaURL: pr,
		}

	case catalog.EventIDERulesSynced:
		repo := getStringDefault(payload, "repoName", "your repository")
		count := getInt(payload, "rulesCount")
		body := fmt.Sprintf("%d rules synced from %s.", count, repo)
		if count == 1 {
			body = fmt.Sprintf("1 rule synced from %s.", repo)
		}
		return application.InAppTemplateResult{
			Title:  "IDE rules synced",
			Body:   body,
			CtaURL: ctaURL,
		}

	case catalog.EventIDERulesSyncFailed:
		repo := getStringDefault(payload, "repoName", "your repository")
		reason := getStringDefault(payload, "reason", "unknown error")
		return application.InAppTemplateResult{
			Title:  "IDE rule sync failed",
			Body:   fmt.Sprintf("Drixy could not sync rules from %s: %s.", repo, reason),
			CtaURL: ctaURL,
		}

	case catalog.EventOrgMemberRemoved:
		org := getStringDefault(payload, "organizationName", "the organization")
		name := "A member"
		if userMap, ok := payload["removedUser"].(map[string]interface{}); ok {
			if n, ok := userMap["name"].(string); ok && n != "" {
				name = n
			} else if e, ok := userMap["email"].(string); ok && e != "" {
				name = e
			}
		}
		return application.InAppTemplateResult{
			Title:  "Member removed",
			Body:   fmt.Sprintf("%s was removed from %s.", name, org),
			CtaURL: ctaURL,
		}

	case catalog.EventOrgRoleChanged:
		affected := getStringDefault(payload, "affectedUserEmail", "A member")
		prev := getStringDefault(payload, "previousRole", "unknown")
		next := getStringDefault(payload, "newRole", "unknown")
		org := getStringDefault(payload, "organizationName", "the organization")
		by := getString(payload, "changedBy")
		body := fmt.Sprintf("%s's role in %s changed from %s to %s.", affected, org, prev, next)
		if by != "" {
			body = fmt.Sprintf("%s's role in %s changed from %s to %s by %s.", affected, org, prev, next, by)
		}
		return application.InAppTemplateResult{
			Title:  "Member role changed",
			Body:   body,
			CtaURL: ctaURL,
		}

	case catalog.EventBillingPaymentFailed:
		amt := getFloat(payload, "amount")
		curr := getStringDefault(payload, "currency", "USD")
		reason := getStringDefault(payload, "failureReason", "declined")
		formatted := fmt.Sprintf("%s %.2f", curr, amt/100.0)
		return application.InAppTemplateResult{
			Title:  "Payment failed",
			Body:   fmt.Sprintf("Your payment of %s could not be processed: %s. Update your payment method to keep your subscription active.", formatted, reason),
			CtaURL: getStringDefault(payload, "updatePaymentUrl", "/billing"),
		}

	case catalog.EventBillingTrialExpiring:
		days := getInt(payload, "daysRemaining")
		rem := fmt.Sprintf("in %d days", days)
		if days == 1 {
			rem = "tomorrow"
		} else if days == 0 {
			rem = "today"
		}
		return application.InAppTemplateResult{
			Title:  "Trial expiring",
			Body:   fmt.Sprintf("Your trial ends %s. Upgrade to keep Drixy reviewing your pull requests.", rem),
			CtaURL: getStringDefault(payload, "upgradeUrl", "/billing/upgrade"),
		}

	case catalog.EventByokLlmErrorsThreshold:
		provider := getStringDefault(payload, "provider", "BYOK")
		count := getInt(payload, "errorCount")
		latest := getStringDefault(payload, "sampleError", "rate limit exceeded")
		return application.InAppTemplateResult{
			Title:  "BYOK LLM errors exceeded threshold",
			Body:   fmt.Sprintf("Your %s model returned %d errors in the recent window. Reviews may be impacted. Latest error: %s.", provider, count, latest),
			CtaURL: "/settings/byok",
		}

	case catalog.EventSpendLimitThresholdReached:
		pct := getInt(payload, "percentage")
		spent := getFloat(payload, "spentUsd")
		limit := getFloat(payload, "monthlyLimitUsd")
		return application.InAppTemplateResult{
			Title:  fmt.Sprintf("BYOK spend at %d%% of monthly limit", pct),
			Body:   fmt.Sprintf("Your BYOK model spend this month is $%.2f of your $%.2f limit (%d%%). Reviews continue to run.", spent, limit, pct),
			CtaURL: "/settings/spend-limits",
		}

	case catalog.EventSpendLimitExceededFinal:
		spent := getFloat(payload, "spentUsd")
		limit := getFloat(payload, "monthlyLimitUsd")
		return application.InAppTemplateResult{
			Title:  "BYOK monthly spend limit exceeded",
			Body:   fmt.Sprintf("Your BYOK spend ($%.2f) has passed your $%.2f monthly limit. Reviews continue to run.", spent, limit),
			CtaURL: "/settings/spend-limits",
		}

	case catalog.EventRuleFileReferencesInvalid:
		count := getInt(payload, "invalidCount")
		repo := getStringDefault(payload, "repoName", "a repository")
		return application.InAppTemplateResult{
			Title:  "Drixy rule references are invalid",
			Body:   fmt.Sprintf("%d rule file references no longer match in %s. Affected rules are skipped during review until fixed.", count, repo),
			CtaURL: "/library/drixy-rules",
		}

	default:
		label := string(event)
		if def, ok := catalog.EventDefaultsMap[event]; ok {
			label = def.Label
		}
		return application.InAppTemplateResult{
			Title:  label,
			Body:   "",
			CtaURL: ctaURL,
		}
	}
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getStringDefault(m map[string]interface{}, key, fallback string) string {
	if val, ok := m[key].(string); ok && val != "" {
		return val
	}
	return fallback
}

func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(int); ok {
		return val
	}
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	if val, ok := m[key].(int); ok {
		return float64(val)
	}
	return 0
}
