package templates

import (
	"fmt"
	"html"
	"time"
)

// TrialExpiringDetails contains metrics and trial expiration information.
type TrialExpiringDetails struct {
	RecipientName string
	OrgName       string
	DaysRemaining int       // e.g. 3
	TrialEndDate  time.Time
	PRsScanned    int       // e.g. 64
	IssuesBlocked int       // e.g. 9
	HoursSaved    float64   // e.g. 14.2
	UpgradeURL    string
}

// RenderTrialExpiring renders a clean reminder when a workspace free trial is expiring soon.
func RenderTrialExpiring(d TrialExpiringDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeOrg := html.EscapeString(d.OrgName)

	days := d.DaysRemaining
	if days <= 0 {
		days = 3
	}

	subject = fmt.Sprintf("%d Days Remaining: Keep your ScanDrix security gates active", days)
	preview := fmt.Sprintf("Your ScanDrix Pro trial for %s expires in %d days.", safeOrg, days)

	endDateStr := "in 3 days"
	if !d.TrialEndDate.IsZero() {
		endDateStr = d.TrialEndDate.UTC().Format("Jan 02, 2006")
	}

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Your 14-day free trial of ScanDrix Pro for <strong>%s</strong> ends on <strong>%s</strong> (%d days remaining).</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">During your trial, Drixy reviewed <strong>%d Pull Requests Scanned</strong> and caught <strong>%d security issues</strong>, saving your team approximately <strong>%.1f hrs</strong> of review time.</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">Upgrade to ScanDrix Pro to keep automated code reviews active without interruption.</p>
	`, safeName, safeOrg, html.EscapeString(endDateStr), days, d.PRsScanned, d.IssuesBlocked, d.HoursSaved)

	htmlBody = RenderBrandLayoutWithHero(preview, "Your ScanDrix trial is ending soon", content, "Upgrade to ScanDrix Pro", d.UpgradeURL, "", DrixyMascotHuggingKnees, "Drixy Trial Notice")
	return subject, htmlBody
}
