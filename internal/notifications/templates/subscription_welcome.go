package templates

import (
	"fmt"
	"html"
	"strings"
)

// RenderSubscriptionWelcome renders a clean welcome email sent immediately upon plan upgrade.
func RenderSubscriptionWelcome(subscriberName, orgName, planTier string, monthlyTokens int64, allocatedModels []string, dashboardURL string) (subject, htmlBody string) {
	normTier := strings.ToUpper(strings.TrimSpace(planTier))
	if normTier == "PRO" {
		normTier = "TEAM"
	}

	planTitle := "Pro Team Plan"
	if normTier == "ENTERPRISE" {
		planTitle = "Enterprise Custom Plan"
	}

	safeSubscriber := html.EscapeString(subscriberName)
	safeOrg := html.EscapeString(orgName)
	safePlanTitle := html.EscapeString(planTitle)

	subject = fmt.Sprintf("Welcome to ScanDrix %s! Your frontier AI models are now unlocked", safePlanTitle)
	preview := fmt.Sprintf("Your %s subscription for %s is now active.", safePlanTitle, safeOrg)

	modelsList := strings.Join(allocatedModels, ", ")

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Thank you for subscribing! Workspace <strong>%s</strong> is now active on the <strong>%s</strong>.</p>
		<p style="margin: 0 0 14px 0; font-size: 14px; color: #ffffff; line-height: 1.6;">
			<strong>Monthly Quota:</strong> %s Tokens<br>
			<strong>Unlocked Models:</strong> %s
		</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">Your team can now run automated reviews and enforce custom security guardrails.</p>
	`, safeSubscriber, safeOrg, safePlanTitle, formatNumber(monthlyTokens), html.EscapeString(modelsList))

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("Your %s is Active", safePlanTitle), content, "Open Workspace Dashboard", dashboardURL, "", DrixyMascotCoolThumbsUp, "Drixy Pro Member")
	return subject, htmlBody
}
