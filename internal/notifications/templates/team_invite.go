package templates

import (
	"fmt"
	"html"
)

// RenderTeamInvite generates the clean white-and-black email inviting a teammate to join a ScanDrix workspace.
func RenderTeamInvite(inviterName, recipientEmail, orgName, role, inviteURL string) (subject, htmlBody string) {
	safeInviter := html.EscapeString(inviterName)
	safeOrg := html.EscapeString(orgName)
	safeRole := html.EscapeString(role)

	subject = fmt.Sprintf("%s invited you to join %s on ScanDrix", safeInviter, safeOrg)
	preview := fmt.Sprintf("You have been invited to join %s on ScanDrix.", safeOrg)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi,</p>
		<p style="margin: 0 0 20px 0;"><strong>%s</strong> has invited you to collaborate on <strong>%s</strong> as a <strong>%s</strong>.</p>
		<p style="margin: 0; font-size: 14px; color: #6b7280;">
			Click the button below to accept the invitation and access the team workspace.
		</p>
	`, safeInviter, safeOrg, safeRole)

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("Join %s on ScanDrix", safeOrg), content, "Accept Invitation", inviteURL)
	return subject, htmlBody
}

// RenderPasswordReset generates a clean white-and-black password reset email.
func RenderPasswordReset(subscriberName, resetURL string) (subject, htmlBody string) {
	name := subscriberName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Reset your ScanDrix Password"
	preview := "A password reset request was received for your ScanDrix account."

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">We received a request to reset your ScanDrix account password.</p>
		<p style="margin: 0 0 20px 0;">Click the button below to set a new password. This link expires in 15 minutes.</p>
		<p style="margin: 0; font-size: 13px; color: #6b7280;">
			If you did not request this, you can safely ignore this email. Your account remains secure.
		</p>
	`, safeName)

	htmlBody = RenderBrandLayout(preview, "Reset your password", content, "Reset Password", resetURL)
	return subject, htmlBody
}
