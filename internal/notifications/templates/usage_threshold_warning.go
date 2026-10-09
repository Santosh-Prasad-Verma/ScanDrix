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

	badgeBg := "rgba(245, 158, 11, 0.15)"
	badgeText := "#fbbf24"
	badgeBorder := "rgba(245, 158, 11, 0.4)"
	barFill := "#f59e0b"
	badgeTitle := fmt.Sprintf("%d%% Quota Used", percent)

	if isExceeded {
		badgeBg = "rgba(239, 68, 68, 0.15)"
		badgeText = "#f87171"
		badgeBorder = "rgba(239, 68, 68, 0.4)"
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
		calloutHTML = RenderCallout(calloutText, "#ef4444", "#111827", "#f87171")
	} else {
		calloutText = fmt.Sprintf(`
			<strong>Approaching Monthly Limit:</strong><br>
			You have <strong>%d%%</strong> remaining until your quota resets on <strong>%s</strong>. We recommend increasing your limits before review queues block.
		`, 100-percent, html.EscapeString(resetStr))
		calloutHTML = RenderCallout(calloutText, "#f59e0b", "#111827", "#fbbf24")
	}

	progressBarHTML := RenderProgressBar(percent, barFill)

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Your workspace <strong>%s</strong> has consumed <strong>%d%%</strong> of its allocated %s quota for this billing cycle.</p>

		<div style="background-color: #111827; border: 1px solid #1e293b; border-radius: 8px; padding: 18px 20px; margin: 18px 0;">
			<div style="display: flex; justify-content: space-between; font-size: 13px; font-weight: 600; color: #ffffff;">
				<span>Usage Summary</span>
				<span style="color: #94a3b8;">%s / %s %s</span>
			</div>
			%s
			<div style="font-size: 12px; color: #94a3b8; margin-top: 6px;">
				Cycle resets on %s
			</div>
		</div>

		%s
	`, badge, safeName, safeOrg, percent, html.EscapeString(unitName), formatNumber(d.UsedUnits), formatNumber(d.LimitUnits), html.EscapeString(unitName), progressBarHTML, html.EscapeString(resetStr), calloutHTML)

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("%d%% Quota Warning", percent), content, "Manage Plan & Add Quota", d.UpgradeURL, "", DrixyMascotPointingGuide, "Drixy Usage Guide")
	return subject, htmlBody
}
