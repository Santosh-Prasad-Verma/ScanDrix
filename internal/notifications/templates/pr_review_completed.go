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

// RenderPRReviewCompleted renders a clean email notification when Drixy finishes reviewing a pull request.
func RenderPRReviewCompleted(d PRReviewCompletedDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeRepo := html.EscapeString(d.RepoName)
	safeTitle := html.EscapeString(d.PRTitle)

	subject = fmt.Sprintf("Drixy Review: %s on %s #%d", d.Verdict, safeRepo, d.PRNumber)
	preview := fmt.Sprintf("Drixy completed code review for PR #%d (%s) on %s: %s", d.PRNumber, safeTitle, safeRepo, d.Verdict)

	verdictUpper := strings.ToUpper(d.Verdict)

	var notesHTML string
	if d.DrixyNotes != "" {
		notesHTML = fmt.Sprintf(`<p style="margin: 0 0 14px 0; font-size: 13px; color: #cbd5e1; line-height: 1.5;">%s</p>`, html.EscapeString(d.DrixyNotes))
	}

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Drixy finished reviewing pull request <strong>#%d</strong> (%s) in <strong>%s</strong>.</p>
		<p style="margin: 0 0 14px 0; font-size: 14px; color: #ffffff; line-height: 1.6;">
			<strong>Verdict:</strong> %s · %d Critical, %d High, %d Suggestions
		</p>
		%s
	`, safeName, d.PRNumber, safeTitle, safeRepo, verdictUpper, d.CriticalCount, d.HighCount, d.SuggestionsCount, notesHTML)

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("PR #%d Review Completed", d.PRNumber), content, "View Review on Pull Request", d.ReviewURL, "", DrixyMascotScanningPages, "Drixy Code Reviewer")
	return subject, htmlBody
}
