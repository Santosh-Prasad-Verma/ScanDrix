package templates

import (
	"fmt"
	"html"
	"strings"
)

// RenderSubscriptionWelcome renders the ambient gradient onboarding email sent immediately upon plan upgrade.
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

	var modelPills strings.Builder
	for _, m := range allocatedModels {
		modelPills.WriteString(fmt.Sprintf(
			`<span style="display: inline-block; background-color: #1e293b; color: #93c5fd; border: 1px solid #334155; font-size: 12px; font-weight: 500; padding: 4px 10px; margin: 3px 6px 3px 0; border-radius: 4px;">%s</span>`,
			html.EscapeString(m),
		))
	}

	tokensStr := formatNumber(monthlyTokens)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 20px 0;">Thank you for subscribing! Your workspace <strong>%s</strong> is now active on the <strong>%s</strong>.</p>

		<div style="background-color: #111827; border: 1px solid #1e293b; border-radius: 8px; padding: 20px 22px; margin: 24px 0;">
			<table border="0" cellpadding="4" cellspacing="0" width="100%%" style="font-size: 14px; color: #cbd5e1;">
				<tr>
					<td style="color: #94a3b8; width: 45%%; padding-bottom: 10px;">Monthly Quota:</td>
					<td style="font-weight: 700; color: #ffffff; padding-bottom: 10px;">%s Tokens</td>
				</tr>
				<tr>
					<td style="color: #94a3b8; vertical-align: top;">Unlocked Models:</td>
					<td>%s</td>
				</tr>
			</table>
		</div>

		<p style="margin: 0; font-size: 14px; color: #94a3b8;">
			Your team can now run automated reviews and enforce custom AST guardrails. If you have any questions, our engineering team is here to help.
		</p>
	`, safeSubscriber, safeOrg, safePlanTitle, tokensStr, modelPills.String())

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("Your %s is Active", safePlanTitle), content, "Open Workspace Dashboard", dashboardURL, "", DrixyMascotCoolThumbsUp, "Drixy Pro Member")
	return subject, htmlBody
}
