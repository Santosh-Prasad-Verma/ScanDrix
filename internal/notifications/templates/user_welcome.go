package templates

import (
	"fmt"
	"html"
)

// RenderNewUserWelcome generates the onboarding email sent when a new account is activated.
func RenderNewUserWelcome(recipientName, dashboardURL string) (subject, htmlBody string) {
	name := recipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Welcome to ScanDrix — Let's secure your code! 🚀"
	preview := "Your ScanDrix account is active and ready."

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0; color: #cbd5e1; line-height: 1.6;">Welcome to ScanDrix! Your account is active. Connect your Git repositories to get started with automated code reviews on every pull request.</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">Check out our Quickstart Guide in the dashboard to invite your team and set up review rules.</p>
	`, safeName)

	htmlBody = RenderBrandLayoutWithBanner(preview, "Welcome to ScanDrix", content, "Open Developer Dashboard", dashboardURL, "", DrixyMascotHappyCelebrating, "Drixy Celebrating", BannerWelcome)
	return subject, htmlBody
}
