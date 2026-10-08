package templates

import (
	"fmt"
	"html"
	"strings"
)

// RenderSubscriptionWelcome renders the clean white-and-black onboarding email sent immediately upon plan upgrade.
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
			`<span style="display: inline-block; background-color: #f3f4f6; color: #111827; font-size: 12px; font-weight: 500; padding: 4px 10px; margin: 3px 6px 3px 0; border-radius: 4px;">%s</span>`,
			html.EscapeString(m),
		))
	}

	tokensStr := formatNumber(monthlyTokens)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 20px 0;">Thank you for subscribing! Your workspace <strong>%s</strong> is now active on the <strong>%s</strong>.</p>

		<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 18px 20px; margin: 24px 0;">
			<table border="0" cellpadding="4" cellspacing="0" width="100%%" style="font-size: 14px; color: #374151;">
				<tr>
					<td style="color: #6b7280; width: 45%%; padding-bottom: 8px;">Monthly Quota:</td>
					<td style="font-weight: 600; color: #111827; padding-bottom: 8px;">%s Tokens</td>
				</tr>
				<tr>
					<td style="color: #6b7280; vertical-align: top;">Unlocked Models:</td>
					<td>%s</td>
				</tr>
			</table>
		</div>

		<p style="margin: 0; font-size: 14px; color: #6b7280;">
			Your team can now run automated reviews and enforce custom AST guardrails. If you have any questions, our engineering team is here to help.
		</p>
	`, safeSubscriber, safeOrg, safePlanTitle, tokensStr, modelPills.String())

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("Your %s is Active", safePlanTitle), content, "Open Workspace Dashboard", dashboardURL)
	return subject, htmlBody
}
