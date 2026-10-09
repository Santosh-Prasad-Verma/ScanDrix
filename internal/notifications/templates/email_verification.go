package templates

import (
	"fmt"
	"html"
)

// RenderEmailVerification generates a clean email verification message for new user signups.
func RenderEmailVerification(recipientName, confirmURL string) (subject, htmlBody string) {
	name := recipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Verify your email to activate ScanDrix"
	preview := "Confirm your email address to activate your ScanDrix account."

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0; color: #cbd5e1; line-height: 1.6;">Welcome to ScanDrix! Please click the button below to confirm your email and activate your account. This link expires in 24 hours.</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8;">If you did not sign up for ScanDrix, you can safely ignore this email.</p>
	`, safeName)

	htmlBody = RenderBrandLayoutWithHero(preview, "Confirm your email address", content, "Verify Email Address", confirmURL, "", DrixyMascotWavingHello, "Drixy")
	return subject, htmlBody
}
