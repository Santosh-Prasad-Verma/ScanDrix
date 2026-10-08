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

// RenderUsageThresholdWarning renders an alert when a workspace hits 80% or 100% usage capacity.
func RenderUsageThresholdWarning(d UsageThresholdDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeOrg := html.EscapeString(d.OrgName)

	unitName := d.UnitType
	if unitName == "" {
		unitName = "tokens"
	}

	percent := d.PercentUsed
	if percent <= 0 {
		percent = 80
	}

	isExceeded := percent >= 100

	badgeBg := "#fffbeb"
	badgeText := "#92400e"
	badgeBorder := "#fde68a"
	barFill := "#f59e0b"
	badgeTitle := fmt.Sprintf("%d%% Quota Used", percent)

	if isExceeded {
		badgeBg = "#fef2f2"
		badgeText = "#991b1b"
		badgeBorder = "#fca5a5"
		barFill = "#ef4444"
		badgeTitle = "100% Quota Exceeded"
	}

	badge := RenderBadge(badgeTitle, badgeBg, badgeText, badgeBorder)

	if isExceeded {
		subject = fmt.Sprintf("Action Required: %s has exceeded monthly review quota (100%%)", safeOrg)
	} else {
		subject = fmt.Sprintf("Notice: %s has reached %d%% of monthly review quota", safeOrg, percent)
	}

	preview := fmt.Sprintf("Workspace %s has consumed %s of %s %s for the current billing period.", safeOrg, formatNumber(d.UsedUnits), formatNumber(d.LimitUnits), unitName)

	resetStr := "the next billing cycle"
	if !d.ResetDate.IsZero() {
		resetStr = d.ResetDate.UTC().Format("Jan 02, 2006")
	}

	var calloutText string
	var calloutHTML string
	if isExceeded {
		calloutText = fmt.Sprintf(`
			<strong>PR Review Queues Paused:</strong><br>
			Your monthly %s quota has been completely used. To avoid delays in CI/CD merges and ensure Drixy continues reviewing pull requests, please upgrade your plan or purchase an additional token pack.
		`, html.EscapeString(unitName))
		calloutHTML = RenderCallout(calloutText, "#fca5a5", "#fef2f2", "#991b1b")
	} else {
		calloutText = fmt.Sprintf(`
			<strong>Approaching Monthly Limit:</strong><br>
			You have <strong>%d%%</strong> remaining until your quota resets on <strong>%s</strong>. We recommend increasing your limits before review queues block.
		`, 100-percent, html.EscapeString(resetStr))
		calloutHTML = RenderCallout(calloutText, "#fde68a", "#fffbeb", "#92400e")
	}

	progressBarHTML := RenderProgressBar(percent, barFill)

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Your workspace <strong>%s</strong> has consumed <strong>%d%%</strong> of its allocated %s quota for this billing cycle.</p>

		<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 16px 18px; margin: 18px 0;">
			<div style="display: flex; justify-content: space-between; font-size: 13px; font-weight: 600; color: #111827;">
				<span>Usage Summary</span>
				<span style="color: #6b7280;">%s / %s %s</span>
			</div>
			%s
			<div style="font-size: 12px; color: #6b7280; margin-top: 6px;">
				Cycle resets on %s
			</div>
		</div>

		%s
	`, badge, safeName, safeOrg, percent, html.EscapeString(unitName), formatNumber(d.UsedUnits), formatNumber(d.LimitUnits), html.EscapeString(unitName), progressBarHTML, html.EscapeString(resetStr), calloutHTML)

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("%d%% Quota Warning", percent), content, "Manage Plan & Add Quota", d.UpgradeURL)
	return subject, htmlBody
}
