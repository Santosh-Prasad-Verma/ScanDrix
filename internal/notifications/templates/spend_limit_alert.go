package templates

import (
	"fmt"
	"html"
)

// RenderSpendLimitAlert alerts workspace admins when monthly token consumption hits thresholds without loud highlight boxes.
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
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Your workspace <strong>%s</strong> has used <strong>%d%%</strong> of its allocated monthly token quota (%s of %s tokens).</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">To prevent code reviews from being paused, you can upgrade your plan or adjust your quota in workspace settings.</p>
	`, safeSubscriber, safeOrg, percent, formatNumber(usedTokens), formatNumber(limitTokens))

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("%d%% of Token Quota Used", percent), content, "Manage Plan & Quota", upgradeURL, "", DrixyMascotShockedAlert, "Drixy Quota Alert")
	return subject, htmlBody
}
