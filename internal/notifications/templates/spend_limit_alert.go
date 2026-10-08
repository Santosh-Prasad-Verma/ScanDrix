package templates

import (
	"fmt"
	"html"
)

// RenderSpendLimitAlert alerts workspace admins when monthly token consumption hits thresholds (e.g. 80% or 100%).
func RenderSpendLimitAlert(subscriberName, orgName string, percent int, usedTokens, limitTokens int64, upgradeURL string) (subject, htmlBody string) {
	safeSubscriber := html.EscapeString(subscriberName)
	safeOrg := html.EscapeString(orgName)

	status := "approaching"
	if percent >= 100 {
		status = "exceeded"
	}
	subject = fmt.Sprintf("Alert: Workspace %s has %s its monthly token quota (%d%%)", safeOrg, status, percent)
	preview := fmt.Sprintf("Workspace %s has consumed %s of %s tokens for this billing period.", safeOrg, formatNumber(usedTokens), formatNumber(limitTokens))

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>Your workspace <strong>%s</strong> has consumed <strong>%d%%</strong> of its allocated monthly token quota.</p>

		<div style="background-color: #1a1711; border: 1px solid #3d2f16; border-left: 4px solid #f59e0b; border-radius: 0 8px 8px 0; padding: 18px 20px; margin: 24px 0;">
			<table border="0" cellpadding="4" cellspacing="0" width="100%%" style="font-size: 14px;">
				<tr>
					<td style="color: #fbbf24; width: 45%%;">Tokens Used:</td>
					<td style="color: #fef3c7; font-weight: 700;">%s Tokens</td>
				</tr>
				<tr>
					<td style="color: #fbbf24;">Monthly Limit:</td>
					<td style="color: #fef3c7; font-weight: 700;">%s Tokens</td>
				</tr>
				<tr>
					<td style="color: #fbbf24;">Threshold:</td>
					<td style="color: #fbbf24; font-weight: 700;">%d%% Cap Reached</td>
				</tr>
			</table>
		</div>

		<p style="color: #a1a1aa; font-size: 14px;">
			When the monthly token cap is reached, automated code reviews may be paused unless you upgrade to an Enterprise plan or supply a Bring-Your-Own-Key (BYOK) API key.
		</p>
	`, safeSubscriber, safeOrg, percent, formatNumber(usedTokens), formatNumber(limitTokens), percent)

	htmlBody = RenderBrandLayout(preview, "Token Quota Alert", content, "Manage Quotas & Top Up", upgradeURL)
	return subject, htmlBody
}
