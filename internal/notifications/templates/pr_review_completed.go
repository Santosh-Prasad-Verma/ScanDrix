package templates

import (
	"fmt"
	"html"
	"strings"
)

// PRReviewCompletedDetails captures summary data when Drixy finishes reviewing a pull request.
type PRReviewCompletedDetails struct {
	RecipientName    string
	OrgName          string
	RepoName         string // e.g. "acme/auth-service"
	PRNumber         int    // e.g. 189
	PRTitle          string // e.g. "feat: implement OAuth2 PKCE token exchange"
	Author           string // e.g. "Sarah Chen"
	Verdict          string // "Approved", "Changes Requested", or "Comments Posted"
	CriticalCount    int
	HighCount        int
	SuggestionsCount int
	DrixyNotes       string // High-level AI review verdict summary
	ReviewURL        string // Direct link to PR review comments on GitHub / GitLab
}

// RenderPRReviewCompleted renders an email notification when Drixy finishes reviewing a pull request.
func RenderPRReviewCompleted(d PRReviewCompletedDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeRepo := html.EscapeString(d.RepoName)
	safeTitle := html.EscapeString(d.PRTitle)
	safeAuthor := html.EscapeString(d.Author)

	verdictLower := strings.ToLower(d.Verdict)
	verdictBg := "#f3f4f6"
	verdictText := "#111827"
	verdictBorder := "#e5e7eb"
	verdictBadge := "REVIEW COMPLETED"

	if strings.Contains(verdictLower, "approve") {
		verdictBg = "#ecfdf5"
		verdictText = "#065f46"
		verdictBorder = "#a7f3d0"
		verdictBadge = "PASSED · APPROVED"
	} else if strings.Contains(verdictLower, "change") || d.CriticalCount > 0 {
		verdictBg = "#fef2f2"
		verdictText = "#991b1b"
		verdictBorder = "#fca5a5"
		verdictBadge = "CHANGES REQUESTED"
	} else if strings.Contains(verdictLower, "comment") || d.SuggestionsCount > 0 {
		verdictBg = "#fffbeb"
		verdictText = "#92400e"
		verdictBorder = "#fde68a"
		verdictBadge = "SUGGESTIONS POSTED"
	}

	subject = fmt.Sprintf("Drixy Review: %s on %s #%d", d.Verdict, safeRepo, d.PRNumber)
	preview := fmt.Sprintf("Drixy completed code review for PR #%d (%s) on %s: %s", d.PRNumber, safeTitle, safeRepo, d.Verdict)

	badge := RenderBadge("Drixy AI Review", "#f3f4f6", "#111827", "#e5e7eb")
	verdictPill := RenderBadge(verdictBadge, verdictBg, verdictText, verdictBorder)

	rows := []InfoRow{
		{Label: "Repository", Value: fmt.Sprintf("<strong>%s</strong>", safeRepo)},
		{Label: "Pull Request", Value: fmt.Sprintf("#%d · %s", d.PRNumber, safeTitle)},
		{Label: "Author", Value: safeAuthor},
	}

	// Findings summary chips
	findingsHTML := fmt.Sprintf(`
		<div style="margin: 16px 0; display: flex; gap: 8px;">
			<span style="display: inline-block; font-size: 11px; font-weight: 700; padding: 3px 8px; border-radius: 4px; background: %s; color: %s; margin-right: 6px;">
				%d Critical
			</span>
			<span style="display: inline-block; font-size: 11px; font-weight: 700; padding: 3px 8px; border-radius: 4px; background: %s; color: %s; margin-right: 6px;">
				%d High
			</span>
			<span style="display: inline-block; font-size: 11px; font-weight: 700; padding: 3px 8px; border-radius: 4px; background: #f3f4f6; color: #374151;">
				%d Suggestions
			</span>
		</div>
	`,
		func() string { if d.CriticalCount > 0 { return "#fee2e2" }; return "#f3f4f6" }(),
		func() string { if d.CriticalCount > 0 { return "#991b1b" }; return "#6b7280" }(),
		d.CriticalCount,
		func() string { if d.HighCount > 0 { return "#ffedd5" }; return "#f3f4f6" }(),
		func() string { if d.HighCount > 0 { return "#9a3412" }; return "#6b7280" }(),
		d.HighCount,
		d.SuggestionsCount,
	)

	// Drixy commentary
	drixyNotesHTML := ""
	if d.DrixyNotes != "" {
		notesMsg := fmt.Sprintf(`<strong>Drixy Summary:</strong><br>%s`, html.EscapeString(d.DrixyNotes))
		drixyNotesHTML = RenderCallout(notesMsg, "#e5e7eb", "#f9fafb", "#374151")
	}

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Drixy has finished analyzing pull request <strong>#%d</strong>:</p>

		<div style="margin-bottom: 8px;">
			%s
		</div>

		%s
		%s
		%s
	`, badge, safeName, d.PRNumber, verdictPill, RenderInfoTable(rows), findingsHTML, drixyNotesHTML)

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("PR Review Completed · #%d", d.PRNumber), content, "View Review on Pull Request", d.ReviewURL)
	return subject, htmlBody
}
