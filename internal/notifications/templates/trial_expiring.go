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

// RenderTrialExpiring renders a reminder when a workspace free trial is 3 days from ending.
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
	preview := fmt.Sprintf("Your ScanDrix Pro trial for %s expires in %d days. Upgrade now to keep continuous code review gates active.", safeOrg, days)

	badge := RenderBadge(fmt.Sprintf("%d Days Left in Trial", days), "rgba(245, 158, 11, 0.15)", "#fbbf24", "rgba(245, 158, 11, 0.4)")

	stats := []StatItem{
		{Value: fmt.Sprintf("%d", d.PRsScanned), Label: "Pull Requests Scanned"},
		{Value: fmt.Sprintf("%d", d.IssuesBlocked), Label: "Vulnerabilities Caught"},
		{Value: fmt.Sprintf("%.1f hrs", d.HoursSaved), Label: "Review Time Saved"},
	}

	endDateStr := "in 3 days"
	if !d.TrialEndDate.IsZero() {
		endDateStr = d.TrialEndDate.UTC().Format("Jan 02, 2006")
	}

	callout := fmt.Sprintf(`
		<strong>What happens when the trial ends?</strong><br>
		On <strong>%s</strong>, automated pull request reviews and CI/CD security blocking gates will pause. Upgrade to ScanDrix Pro to maintain uninterrupted protection and retain team settings.
	`, html.EscapeString(endDateStr))
	calloutHTML := RenderCallout(callout, "#f59e0b", "#111827", "#cbd5e1")

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Your 14-day free trial of ScanDrix Pro for <strong>%s</strong> will end on <strong>%s</strong>.</p>

		<p style="margin: 20px 0 6px 0; font-size: 13px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; color: #94a3b8;">
			Your Team's Trial Impact
		</p>
		%s
		%s
	`, badge, safeName, safeOrg, html.EscapeString(endDateStr), RenderStatGrid(stats), calloutHTML)

	footnote := `
		<p style="margin: 16px 0 0 0; font-size: 13px; color: #94a3b8;">
			Need custom invoicing, SAML SSO, or an extended evaluation? Reply to this email or reach out to <a href="mailto:support@scandrix.dev" style="color: #818cf8; text-decoration: underline;">support@scandrix.dev</a>.
		</p>
	`

	htmlBody = RenderBrandLayoutWithHero(preview, "Your ScanDrix trial is ending soon", content, "Upgrade to ScanDrix Pro", d.UpgradeURL, footnote, DrixyMascotHuggingKnees, "Drixy Trial Notice")
	return subject, htmlBody
}
