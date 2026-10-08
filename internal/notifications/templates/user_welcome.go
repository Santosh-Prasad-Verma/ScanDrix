package templates

import (
	"fmt"
	"html"
)

// RenderNewUserWelcome generates the clean white-and-black onboarding email sent when a new account is activated.
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

		<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 18px 20px; margin: 24px 0;">
			<div style="font-size: 13px; font-weight: 600; color: #111827; margin-bottom: 8px;">Quickstart Guide</div>
			<div style="font-size: 14px; color: #4b5563; line-height: 1.6;">
				1. Connect your GitHub or GitLab repositories in workspace settings.<br>
				2. Open a pull request to trigger your first automated AI review.<br>
				3. Optional: Install the CLI via <code style="background-color: #f3f4f6; padding: 2px 6px; border-radius: 4px; font-family: monospace; font-size: 12px;">curl -fsSL https://get.scandrix.dev/install | sh</code>
			</div>
		</div>

		<p style="margin: 0; font-size: 14px; color: #6b7280;">
			Need help getting set up? Reply directly to this email or visit our <a href="https://scandrix.dev/docs" style="color: #111827; text-decoration: underline;">documentation</a>.
		</p>
	`, safeName)

	htmlBody = RenderBrandLayout(preview, "Welcome to ScanDrix", content, "Open Developer Dashboard", dashboardURL)
	return subject, htmlBody
}
