package templates

import (
	"fmt"
	"html"
	"time"
)

// UsageThresholdDetails holds data when a workspace reaches 80% or 100% quota consumption.
type UsageThresholdDetails struct {
	RecipientName string
	OrgName       string
	PlanTier      string    // e.g. "Pro Team"
	PercentUsed   int       // 80 or 100
	UsedUnits     int64     // e.g. 40,000,000
	LimitUnits    int64     // e.g. 50,000,000
	UnitType      string    // "Tokens" or "PR Scans"
	ResetDate     time.Time // Next billing cycle renewal date
	UpgradeURL    string
}

// RenderUsageThresholdWarning renders a clean alert when a workspace hits usage thresholds.
func RenderUsageThresholdWarning(d UsageThresholdDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeOrg := html.EscapeString(d.OrgName)

	unitName := d.UnitType
	if unitName == "" {
		unitName = "Tokens"
	}

	percent := d.PercentUsed
	if percent <= 0 {
		percent = 80
	}

	isExceeded := percent >= 100

	if isExceeded {
		subject = fmt.Sprintf("Action Required: %s has exceeded monthly review quota (100%%)", safeOrg)
	} else {
		subject = fmt.Sprintf("Notice: %s has reached %d%% of monthly review quota", safeOrg, percent)
	}

	preview := fmt.Sprintf("Workspace %s has consumed %s of %s %s.", safeOrg, formatNumber(d.UsedUnits), formatNumber(d.LimitUnits), unitName)

	resetStr := "the next billing cycle"
	if !d.ResetDate.IsZero() {
		resetStr = d.ResetDate.UTC().Format("Jan 02, 2006")
	}

	statusText := fmt.Sprintf("%d%% of monthly %s quota used (%s / %s %s).", percent, html.EscapeString(unitName), formatNumber(d.UsedUnits), formatNumber(d.LimitUnits), html.EscapeString(unitName))
	if isExceeded {
		statusText = fmt.Sprintf("100%% Quota Exceeded (%s / %s %s used).", formatNumber(d.UsedUnits), formatNumber(d.LimitUnits), html.EscapeString(unitName))
	}

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Your workspace <strong>%s</strong> status: <strong>%s</strong></p>
		<p style="margin: 0 0 14px 0; font-size: 13px; color: #94a3b8;">Cycle resets on %s.</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">Upgrade your plan or adjust quota settings to ensure continuous code reviews.</p>
	`, safeName, safeOrg, statusText, html.EscapeString(resetStr))

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("%d%% Quota Notice", percent), content, "Manage Plan & Add Quota", d.UpgradeURL, "", DrixyMascotPointingGuide, "Drixy Usage Guide")
	return subject, htmlBody
}
