package templates

import (
	"fmt"
	"html"
)

// RenderTeamInvite generates the ambient gradient email inviting a teammate to join a ScanDrix workspace.
func RenderTeamInvite(inviterName, recipientEmail, orgName, role, inviteURL string) (subject, htmlBody string) {
	safeInviter := html.EscapeString(inviterName)
	safeOrg := html.EscapeString(orgName)
	safeRole := html.EscapeString(role)

	subject = fmt.Sprintf("%s invited you to join %s on ScanDrix", safeInviter, safeOrg)
	preview := fmt.Sprintf("You have been invited to join %s on ScanDrix.", safeOrg)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi,</p>
		<p style="margin: 0 0 20px 0;"><strong>%s</strong> has invited you to collaborate on <strong>%s</strong> as a <strong>%s</strong>.</p>
		<p style="margin: 0; font-size: 14px; color: #94a3b8;">
			Click the button below to accept the invitation and access the team workspace.
		</p>
	`, safeInviter, safeOrg, safeRole)

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("Join %s on ScanDrix", safeOrg), content, "Accept Invitation", inviteURL, "", DrixyMascotFloatingWaving, "Drixy Workspace Invitation")
	return subject, htmlBody
}
