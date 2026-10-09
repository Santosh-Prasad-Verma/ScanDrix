package templates

import (
	"fmt"
	"html"
)

// RenderEmailVerification generates the ambient gradient email verification message for new user signups.
func RenderEmailVerification(recipientName, confirmURL string) (subject, htmlBody string) {
	name := recipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeURL := html.EscapeString(confirmURL)

	subject = "Verify your email to activate ScanDrix"
	preview := "Confirm your email address to activate your ScanDrix account."

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">Welcome to ScanDrix! Please click the button below to confirm your email and activate your automated code review workspace.</p>
	`, safeName)

	footnote := fmt.Sprintf(`
		<div style="font-size: 13px; color: #94a3b8; line-height: 1.55; margin-top: 14px;">
			This verification link expires in 24 hours. If you did not create a ScanDrix account, you can safely ignore this email.
			<div style="margin-top: 8px; font-size: 12px; color: #64748b; word-break: break-all;">
				Link: <a href="%s" style="color: #818cf8; text-decoration: underline;">%s</a>
			</div>
		</div>
	`, safeURL, safeURL)

	htmlBody = RenderBrandLayoutWithHero(preview, "Confirm your email address", content, "Verify Email Address", confirmURL, footnote, DrixyMascotWavingHello, "Drixy Welcome Mascot")
	return subject, htmlBody
}
