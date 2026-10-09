package templates

import (
	"fmt"
	"html"
)

// RenderNewUserWelcome generates the ambient gradient onboarding email sent when a new account is activated.
func RenderNewUserWelcome(recipientName, dashboardURL string) (subject, htmlBody string) {
	name := recipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Welcome to ScanDrix — Let's secure your code! 🚀"
	preview := "Your ScanDrix account is active and ready."

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 20px 0;">Welcome to ScanDrix! Your account is active and ready. You can now connect your Git repositories to start automated AI code reviews on your pull requests.</p>

		<div style="background-color: #111827; border: 1px solid #1e293b; border-radius: 8px; padding: 20px 22px; margin: 24px 0;">
			<div style="font-size: 14px; font-weight: 700; color: #ffffff; margin-bottom: 10px;">Quickstart Guide</div>
			<div style="font-size: 14px; color: #cbd5e1; line-height: 1.65;">
				1. Connect your GitHub, GitLab, or Forgejo repositories in workspace settings.<br>
				2. Open a pull request to trigger your first automated AI review.<br>
				3. Optional: Install the CLI via <code style="background-color: #060710; color: #38bdf8; padding: 2px 6px; border-radius: 4px; font-family: monospace; font-size: 12px; border: 1px solid #1e293b;">curl -fsSL https://get.scandrix.dev/install | sh</code>
			</div>
		</div>

		<p style="margin: 0; font-size: 14px; color: #94a3b8;">
			Need help getting set up? Reply directly to this email or visit our <a href="https://scandrix.dev/docs" style="color: #818cf8; text-decoration: underline;">documentation</a>.
		</p>
	`, safeName)

	htmlBody = RenderBrandLayoutWithHero(preview, "Welcome to ScanDrix", content, "Open Developer Dashboard", dashboardURL, "", DrixyMascotHappyCelebrating, "Drixy Celebrating")
	return subject, htmlBody
}
