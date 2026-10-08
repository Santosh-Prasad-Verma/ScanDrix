package templates

import (
	"fmt"
	"html"
)

// RenderNewUserWelcome generates the onboarding welcome email sent when a new account is activated.
func RenderNewUserWelcome(recipientName, dashboardURL string) (subject, htmlBody string) {
	name := recipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Welcome to ScanDrix — Let's secure your code! 🚀"
	preview := "Your ScanDrix account is active. Here is how to get started in 60 seconds."

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>
			Your <strong>ScanDrix</strong> account is officially verified and active! You're ready to bring autonomous, multi-agent AI code reviews and continuous AST security scanning directly to your pull requests.
		</p>

		<div style="background-color: #16171b; border: 1px solid #26282f; border-radius: 8px; padding: 22px; margin: 24px 0;">
			<h3 style="margin: 0 0 16px 0; color: #ffffff; font-size: 15px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em;">Quickstart Guide:</h3>
			<table border="0" cellpadding="0" cellspacing="0" width="100%%" style="font-size: 14px; line-height: 1.6;">
				<tr>
					<td style="vertical-align: top; width: 32px; padding-bottom: 14px;">
						<span style="display:inline-block; width:22px; height:22px; line-height:22px; text-align:center; background-color:rgba(201,243,107,0.15); color:#c9f36b; font-weight:700; border-radius:4px; border:1px solid rgba(201,243,107,0.3); font-size:12px;">1</span>
					</td>
					<td style="padding-bottom: 14px; color: #d4d4d8;">
						<strong style="color: #ffffff;">Connect Your Repositories:</strong> Link your GitHub, GitLab, Bitbucket, or Azure DevOps repos in your workspace settings for automatic PR review bots.
					</td>
				</tr>
				<tr>
					<td style="vertical-align: top; width: 32px; padding-bottom: 14px;">
						<span style="display:inline-block; width:22px; height:22px; line-height:22px; text-align:center; background-color:rgba(201,243,107,0.15); color:#c9f36b; font-weight:700; border-radius:4px; border:1px solid rgba(201,243,107,0.3); font-size:12px;">2</span>
					</td>
					<td style="padding-bottom: 14px; color: #d4d4d8;">
						<strong style="color: #ffffff;">Install the ScanDrix CLI:</strong> Run security audits directly in your terminal before pushing:<br>
						<code style="display: inline-block; background-color: #080807; color: #c9f36b; padding: 6px 12px; border-radius: 6px; font-family: monospace; font-size: 13px; margin-top: 6px; border: 1px solid #26282f;">curl -fsSL https://get.scandrix.dev/install | sh</code>
					</td>
				</tr>
				<tr>
					<td style="vertical-align: top; width: 32px;">
						<span style="display:inline-block; width:22px; height:22px; line-height:22px; text-align:center; background-color:rgba(201,243,107,0.15); color:#c9f36b; font-weight:700; border-radius:4px; border:1px solid rgba(201,243,107,0.3); font-size:12px;">3</span>
					</td>
					<td style="color: #d4d4d8;">
						<strong style="color: #ffffff;">Define Drixy Rules:</strong> Enforce custom AST policy-as-code rules, architecture boundaries, and taint flow analysis.
					</td>
				</tr>
			</table>
		</div>

		<p style="color: #a1a1aa; font-size: 14px;">
			Have questions or need enterprise onboarding support? Explore our <a href="https://scandrix.dev/docs" style="color: #c9f36b; text-decoration: underline;">documentation</a> or reply directly to this email to reach our team.
		</p>
	`, safeName)

	htmlBody = RenderBrandLayout(preview, "Welcome to ScanDrix", content, "Open Developer Dashboard", dashboardURL)
	return subject, htmlBody
}
