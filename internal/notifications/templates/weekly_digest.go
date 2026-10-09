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

// RenderWeeklyScanDigest generates a clean weekly security digest email without loud stat grids or badges.
func RenderWeeklyScanDigest(d WeeklyScanDigestDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeOrg := html.EscapeString(d.OrgName)
	safeDateRange := html.EscapeString(d.WeekDateRange)

	subject = fmt.Sprintf("Weekly Security Digest: %s · %d PRs Reviewed, %d Blocked", safeOrg, d.TotalPRsReviewed, d.VulnerabilitiesBlocked)
	preview := fmt.Sprintf("Weekly ScanDrix summary for %s: %d PRs reviewed.", safeOrg, d.TotalPRsReviewed)

	var repoNames []string
	for _, r := range d.TopRepos {
		repoNames = append(repoNames, html.EscapeString(r.Repository))
	}
	topReposStr := ""
	if len(repoNames) > 0 {
		topReposStr = fmt.Sprintf(`<p style="margin: 0 0 14px 0; font-size: 13px; color: #94a3b8;">Top active repositories: %s</p>`, strings.Join(repoNames, ", "))
	}

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Here is your weekly summary for <strong>%s</strong> (%s):</p>
		<p style="margin: 0 0 14px 0; font-size: 14px; color: #ffffff; line-height: 1.8;">
			• <strong>%d</strong> PRs Reviewed by Drixy<br>
			• <strong>%d</strong> security issues blocked (%d critical)<br>
			• <strong>%d%%</strong> workspace health score<br>
			• <strong>%.1f hrs</strong> review time saved
		</p>
		%s
	`, safeName, safeOrg, safeDateRange, d.TotalPRsReviewed, d.VulnerabilitiesBlocked, d.CriticalBlocked, d.HealthScore, d.HoursSaved, topReposStr)

	htmlBody = RenderBrandLayoutWithHero(preview, "Weekly Security & Review Digest", content, "View Weekly Dashboard", d.DashboardURL, "", DrixyMascotCoffeeBreak, "Drixy Coffee Break")
	return subject, htmlBody
}
