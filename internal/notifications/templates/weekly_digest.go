package templates

import (
	"fmt"
	"html"
	"strings"
)

// DigestRepoItem represents one repository's metrics in the weekly digest.
type DigestRepoItem struct {
	Repository    string
	PRsScanned    int
	IssuesBlocked int
	HealthGrade   string // "A+", "A", "B", etc.
}

// WeeklyScanDigestDetails holds the weekly aggregated metrics for a workspace.
type WeeklyScanDigestDetails struct {
	RecipientName          string
	OrgName                string
	WeekDateRange          string // e.g. "Oct 01 - Oct 07, 2026"
	TotalPRsReviewed       int    // e.g. 48
	VulnerabilitiesBlocked int    // e.g. 14
	CriticalBlocked        int    // e.g. 3
	HoursSaved             float64 // e.g. 18.5
	HealthScore            int    // e.g. 98 (%)
	TopRepos               []DigestRepoItem
	DashboardURL           string
}

// RenderWeeklyScanDigest generates the Monday morning weekly security digest email.
func RenderWeeklyScanDigest(d WeeklyScanDigestDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeOrg := html.EscapeString(d.OrgName)
	safeDateRange := html.EscapeString(d.WeekDateRange)

	subject = fmt.Sprintf("Weekly Security Digest: %s · %d PRs Reviewed, %d Blocked", safeOrg, d.TotalPRsReviewed, d.VulnerabilitiesBlocked)
	preview := fmt.Sprintf("Weekly ScanDrix summary for %s: %d pull requests reviewed by Drixy, %d security issues blocked.", safeOrg, d.TotalPRsReviewed, d.VulnerabilitiesBlocked)

	badge := RenderBadge("Weekly Digest", "#f3f4f6", "#111827", "#e5e7eb")

	stats := []StatItem{
		{Value: fmt.Sprintf("%d", d.TotalPRsReviewed), Label: "PRs Reviewed by Drixy"},
		{Value: fmt.Sprintf("%d", d.VulnerabilitiesBlocked), Label: fmt.Sprintf("Issues Blocked (%d Crit)", d.CriticalBlocked)},
		{Value: fmt.Sprintf("%d%%", d.HealthScore), Label: "Security Health Score"},
		{Value: fmt.Sprintf("%.1f hrs", d.HoursSaved), Label: "Review Time Saved"},
	}

	// Render Top Repositories table
	var reposHTML string
	if len(d.TopRepos) > 0 {
		var rows strings.Builder
		rows.WriteString(`
			<div style="margin: 24px 0 10px 0;">
				<span style="font-size: 13px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; color: #374151;">
					Top Scanned Repositories
				</span>
			</div>
			<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; border-collapse: separate; overflow: hidden; margin-bottom: 20px;">
		`)

		for i, repo := range d.TopRepos {
			borderBottom := "border-bottom: 1px solid #f3f4f6;"
			if i == len(d.TopRepos)-1 {
				borderBottom = ""
			}
			grade := repo.HealthGrade
			if grade == "" {
				grade = "A"
			}
			rows.WriteString(fmt.Sprintf(`
				<tr>
					<td style="padding: 10px 16px; font-size: 13px; font-weight: 600; color: #111827; %s">
						%s
					</td>
					<td style="padding: 10px 16px; font-size: 12px; color: #6b7280; text-align: right; %s">
						%d PRs · %d blocked
						<span style="display: inline-block; margin-left: 8px; font-size: 10px; font-weight: 700; background: #e5e7eb; color: #111827; padding: 2px 6px; border-radius: 3px;">%s</span>
					</td>
				</tr>
			`, borderBottom, html.EscapeString(repo.Repository), borderBottom, repo.PRsScanned, repo.IssuesBlocked, html.EscapeString(grade)))
		}
		rows.WriteString(`</table>`)
		reposHTML = rows.String()
	}

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Here is your weekly code review and security digest for <strong>%s</strong> covering <strong>%s</strong>:</p>

		%s
		%s

		<p style="margin: 16px 0 0 0; font-size: 14px; color: #4b5563;">
			Drixy continuously inspects every incoming pull request to safeguard code quality, prevent vulnerabilities, and save engineering hours.
		</p>
	`, badge, safeName, safeOrg, safeDateRange, RenderStatGrid(stats), reposHTML)

	htmlBody = RenderBrandLayout(preview, "Weekly Security & Review Digest", content, "View Weekly Dashboard", d.DashboardURL)
	return subject, htmlBody
}
