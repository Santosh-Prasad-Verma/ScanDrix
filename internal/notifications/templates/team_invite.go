package templates

import (
	"fmt"
	"html"
)

// RenderTeamInvite generates the email inviting a teammate to join a ScanDrix workspace.
func RenderTeamInvite(inviterName, recipientEmail, orgName, role, inviteURL string) (subject, htmlBody string) {
	safeInviter := html.EscapeString(inviterName)
	safeOrg := html.EscapeString(orgName)
	safeRole := html.EscapeString(role)

	subject = fmt.Sprintf("%s invited you to join %s on ScanDrix", safeInviter, safeOrg)
	preview := fmt.Sprintf("You have been invited to join %s on the ScanDrix automated code review platform.", safeOrg)

	content := fmt.Sprintf(`
		<p>Hi there,</p>
		<p><strong>%s</strong> has invited you to collaborate on <strong>%s</strong> as a <strong>%s</strong> on ScanDrix.</p>

		<p style="color: #4b5563;">
			ScanDrix provides automated AI code reviews, multi-agent deliberation, and continuous security AST analysis directly on your pull requests.
		</p>
	`, safeInviter, safeOrg, safeRole)

	htmlBody = RenderBrandLayout(preview, "Join Your Team on ScanDrix", content, "Accept Invitation", inviteURL)
	return subject, htmlBody
}

// RenderPasswordReset generates a password reset email using the brand layout.
func RenderPasswordReset(subscriberName, resetURL string) (subject, htmlBody string) {
	name := subscriberName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Reset your ScanDrix Password"
	preview := "A password reset request was received for your ScanDrix account."

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>We received a request to reset the password for your ScanDrix account.</p>
		<p>Click the button below to choose a new password. This link is cryptographically signed and valid for <strong>15 minutes</strong>.</p>
		<p style="color: #64748b; font-size: 13px; margin-top: 20px;">
			If you did not initiate this request, you can safely ignore this email. Your credentials remain secure.
		</p>
	`, safeName)

	htmlBody = RenderBrandLayout(preview, "Password Reset Request", content, "Reset Password", resetURL)
	return subject, htmlBody
}
