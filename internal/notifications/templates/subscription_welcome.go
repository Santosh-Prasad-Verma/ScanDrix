package templates

import (
	"fmt"
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

	subject = fmt.Sprintf("Welcome to ScanDrix %s! Your frontier AI models are now unlocked", planTitle)
	preview := fmt.Sprintf("Your %s subscription for %s is now active. Enjoy frontier model access and elevated token limits.", planTitle, orgName)

	var modelPills strings.Builder
	for _, m := range allocatedModels {
		modelPills.WriteString(fmt.Sprintf(
			`<span style="display: inline-block; background-color: #f3f4f6; color: #374151; font-size: 12px; font-weight: 600; padding: 4px 10px; margin: 3px 4px 3px 0; border-radius: 9999px; border: 1px solid #e5e7eb;">%s</span>`,
			m,
		))
	}

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>Thank you for subscribing! Your workspace <strong>%s</strong> has been successfully upgraded to the <strong>%s</strong>.</p>

		<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 8px; padding: 20px; margin: 24px 0;">
			<h3 style="margin: 0 0 12px 0; color: #111827; font-size: 15px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em;">
				Plan Entitlements &amp; Capabilities
			</h3>
			<table border="0" cellpadding="6" cellspacing="0" width="100%%" style="font-size: 14px;">
				<tr>
					<td style="color: #6b7280; width: 45%%;">Monthly Token Quota:</td>
					<td style="color: #111827; font-weight: 600;">%s Tokens</td>
				</tr>
				<tr>
					<td style="color: #6b7280;">High-Speed Burst Limit:</td>
					<td style="color: #111827; font-weight: 600;">500,000 Tokens / min</td>
				</tr>
				<tr>
					<td style="color: #6b7280;">Concurrent PR Reviews:</td>
					<td style="color: #111827; font-weight: 600;">10 Parallel Workers</td>
				</tr>
				<tr>
					<td style="color: #6b7280;">Enterprise Features:</td>
					<td style="color: #111827; font-weight: 600;">SAML SSO, SCIM, Custom Rules, Proof-of-Fix</td>
				</tr>
			</table>
		</div>

		<h4 style="margin: 20px 0 8px 0; color: #374151; font-size: 14px;">Unlocked Frontier AI Models:</h4>
		<div style="margin-bottom: 24px;">
			%s
		</div>

		<p style="color: #4b5563;">
			All team pull requests submitted to your connected GitHub, GitLab, Bitbucket, and Azure DevOps repositories will now automatically leverage these frontier review engines.
		</p>
	`, subscriberName, orgName, planTitle, formatNumber(monthlyTokens), modelPills.String())

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("Welcome to ScanDrix %s", planTitle), content, "Open Developer Dashboard", dashboardURL)
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
