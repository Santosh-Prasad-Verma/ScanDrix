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
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Your workspace <strong>%s</strong> has used <strong>%d%%</strong> of its allocated monthly token quota.</p>

		<div style="background-color: #fffbeb; border: 1px solid #fef3c7; border-left: 3px solid #f59e0b; border-radius: 4px; padding: 14px 18px; margin: 20px 0; font-size: 14px; color: #92400e;">
			<strong>Tokens Used:</strong> %s of %s tokens
		</div>

		<p style="margin: 0; font-size: 14px; color: #6b7280;">
			To prevent automated PR reviews from being paused, you can upgrade your plan or adjust your quota settings.
		</p>
	`, safeSubscriber, safeOrg, percent, formatNumber(usedTokens), formatNumber(limitTokens))

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("%d%% of Token Quota Used", percent), content, "Manage Plan & Quota", upgradeURL)
	return subject, htmlBody
}
