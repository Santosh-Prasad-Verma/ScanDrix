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

// RenderPRReviewCompleted renders an ambient gradient email notification when Drixy finishes reviewing a pull request.
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
	verdictBg := "rgba(99, 102, 241, 0.15)"
	verdictText := "#c7d2fe"
	verdictBorder := "rgba(99, 102, 241, 0.35)"
	verdictBadge := "REVIEW COMPLETED"

	if strings.Contains(verdictLower, "approve") {
		verdictBg = "rgba(16, 185, 129, 0.15)"
		verdictText = "#34d399"
		verdictBorder = "rgba(16, 185, 129, 0.4)"
		verdictBadge = "PASSED · APPROVED"
	} else if strings.Contains(verdictLower, "change") || d.CriticalCount > 0 {
		verdictBg = "rgba(239, 68, 68, 0.15)"
		verdictText = "#f87171"
		verdictBorder = "rgba(239, 68, 68, 0.4)"
		verdictBadge = "CHANGES REQUESTED"
	} else if strings.Contains(verdictLower, "comment") || d.SuggestionsCount > 0 {
		verdictBg = "rgba(245, 158, 11, 0.15)"
		verdictText = "#fbbf24"
		verdictBorder = "rgba(245, 158, 11, 0.4)"
		verdictBadge = "SUGGESTIONS POSTED"
	}

	subject = fmt.Sprintf("Drixy Review: %s on %s #%d", d.Verdict, safeRepo, d.PRNumber)
	preview := fmt.Sprintf("Drixy completed code review for PR #%d (%s) on %s: %s", d.PRNumber, safeTitle, safeRepo, d.Verdict)

	badge := RenderBadge("Drixy AI Review", "rgba(99, 102, 241, 0.15)", "#c7d2fe", "rgba(99, 102, 241, 0.35)")
	verdictPill := RenderBadge(verdictBadge, verdictBg, verdictText, verdictBorder)

	rows := []InfoRow{
		{Label: "Repository", Value: fmt.Sprintf("<strong>%s</strong>", safeRepo)},
		{Label: "Pull Request", Value: fmt.Sprintf("#%d · %s", d.PRNumber, safeTitle)},
		{Label: "Author", Value: safeAuthor},
	}

	// Findings summary chips
	findingsHTML := fmt.Sprintf(`
		<div style="margin: 16px 0;">
			<span style="display: inline-block; font-size: 11px; font-weight: 700; padding: 4px 10px; border-radius: 4px; background: %s; color: %s; border: 1px solid %s; margin-right: 6px;">
				%d Critical
			</span>
			<span style="display: inline-block; font-size: 11px; font-weight: 700; padding: 4px 10px; border-radius: 4px; background: %s; color: %s; border: 1px solid %s; margin-right: 6px;">
				%d High
			</span>
			<span style="display: inline-block; font-size: 11px; font-weight: 700; padding: 4px 10px; border-radius: 4px; background: #1e293b; color: #94a3b8; border: 1px solid #334155;">
				%d Suggestions
			</span>
		</div>
	`,
		func() string { if d.CriticalCount > 0 { return "rgba(239, 68, 68, 0.15)" }; return "#1e293b" }(),
		func() string { if d.CriticalCount > 0 { return "#f87171" }; return "#94a3b8" }(),
		func() string { if d.CriticalCount > 0 { return "rgba(239, 68, 68, 0.4)" }; return "#334155" }(),
		d.CriticalCount,
		func() string { if d.HighCount > 0 { return "rgba(245, 158, 11, 0.15)" }; return "#1e293b" }(),
		func() string { if d.HighCount > 0 { return "#fbbf24" }; return "#94a3b8" }(),
		func() string { if d.HighCount > 0 { return "rgba(245, 158, 11, 0.4)" }; return "#334155" }(),
		d.HighCount,
		d.SuggestionsCount,
	)

	// Drixy commentary
	drixyNotesHTML := ""
	if d.DrixyNotes != "" {
		notesMsg := fmt.Sprintf(`<strong>Drixy Summary:</strong><br>%s`, html.EscapeString(d.DrixyNotes))
		drixyNotesHTML = RenderCallout(notesMsg, "#6366f1", "#111827", "#cbd5e1")
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

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("PR Review Completed · #%d", d.PRNumber), content, "View Review on Pull Request", d.ReviewURL, "", DrixyMascotScanningPages, "Drixy Code Reviewer")
	return subject, htmlBody
}
