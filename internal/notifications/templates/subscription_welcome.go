package templates

import (
	"fmt"
	"html"
	"strings"
)

// RenderSubscriptionWelcome renders the onboarding email sent immediately upon plan upgrade.
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
	preview := fmt.Sprintf("Your %s subscription for %s is now active. Enjoy frontier model access and elevated token limits.", safePlanTitle, safeOrg)

	var modelPills strings.Builder
	for _, m := range allocatedModels {
		modelPills.WriteString(fmt.Sprintf(
			`<span style="display: inline-block; background-color: #16171b; color: #ffffff; font-size: 12px; font-weight: 600; padding: 6px 12px; margin: 4px 6px 4px 0; border-radius: 6px; border: 1px solid #26282f;"><span style="color: #c9f36b; margin-right: 4px;">&#9679;</span>%s</span>`,
			html.EscapeString(m),
		))
	}

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>Thank you for subscribing! Your workspace <strong>%s</strong> has been successfully upgraded to the <strong>%s</strong>.</p>

		<div style="background-color: #16171b; border: 1px solid #26282f; border-radius: 8px; padding: 22px; margin: 24px 0;">
			<h3 style="margin: 0 0 14px 0; color: #ffffff; font-size: 14px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em;">
				Plan Entitlements &amp; Capabilities
			</h3>
			<table border="0" cellpadding="6" cellspacing="0" width="100%%" style="font-size: 14px;">
				<tr>
					<td style="color: #a1a1aa; width: 45%%;">Monthly Token Quota:</td>
					<td style="color: #ffffff; font-weight: 700;">%s Tokens</td>
				</tr>
				<tr>
					<td style="color: #a1a1aa;">High-Speed Burst Limit:</td>
					<td style="color: #ffffff; font-weight: 600;">500,000 Tokens / min</td>
				</tr>
				<tr>
					<td style="color: #a1a1aa;">Concurrent PR Reviews:</td>
					<td style="color: #ffffff; font-weight: 600;">10 Parallel Workers</td>
				</tr>
				<tr>
					<td style="color: #a1a1aa;">Enterprise Features:</td>
					<td style="color: #c9f36b; font-weight: 600;">SAML SSO, SCIM, Custom Rules, Proof-of-Fix</td>
				</tr>
			</table>
		</div>

		<h4 style="margin: 20px 0 10px 0; color: #ffffff; font-size: 14px; font-weight: 700;">Unlocked Frontier AI Models:</h4>
		<div style="margin-bottom: 24px;">
			%s
		</div>

		<p style="color: #a1a1aa; font-size: 14px;">
			All team pull requests submitted to your connected GitHub, GitLab, Bitbucket, and Azure DevOps repositories will now automatically leverage these frontier review engines.
		</p>
	`, safeSubscriber, safeOrg, safePlanTitle, formatNumber(monthlyTokens), modelPills.String())

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("Welcome to ScanDrix %s", safePlanTitle), content, "Open Developer Dashboard", dashboardURL)
	return subject, htmlBody
}

func formatNumber(n int64) string {
	in := fmt.Sprintf("%d", n)
	var out []rune
	for i, r := range in {
		if i > 0 && (len(in)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, r)
	}
	return string(out)
}
