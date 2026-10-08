package templates

import (
	"fmt"
	"html"
)

// RenderEmailVerification generates the clean white-and-black email verification message for new user signups.
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
		<p style="margin: 0;">Please click the button below to confirm your email and activate your ScanDrix account.</p>
	`, safeName)

	footnote := fmt.Sprintf(`
		<div style="font-size: 13px; color: #6b7280; line-height: 1.5; margin-top: 10px;">
			This verification link expires in 24 hours. If you did not create a ScanDrix account, you can safely ignore this email.
			<div style="margin-top: 8px; font-size: 12px; color: #9ca3af; word-break: break-all;">
				Link: <a href="%s" style="color: #6b7280; text-decoration: underline;">%s</a>
			</div>
		</div>
	`, safeURL, safeURL)

	htmlBody = RenderBrandLayoutWithFootnote(preview, "Confirm your email address", content, "Verify Email Address", confirmURL, footnote)
	return subject, htmlBody
}
