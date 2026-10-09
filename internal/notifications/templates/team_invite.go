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
	preview := fmt.Sprintf("You have been invited to join %s on ScanDrix.", safeOrg)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi,</p>
		<p style="margin: 0 0 16px 0; color: #cbd5e1; line-height: 1.6;"><strong>%s</strong> has invited you to join <strong>%s</strong> on ScanDrix as a <strong>%s</strong>.</p>
	`, safeInviter, safeOrg, safeRole)

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("Join %s on ScanDrix", safeOrg), content, "Accept Invitation", inviteURL, "", DrixyMascotFloatingWaving, "Drixy")
	return subject, htmlBody
}
