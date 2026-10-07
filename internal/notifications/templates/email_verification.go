package templates

import (
	"fmt"
	"html"
)

// RenderEmailVerification generates the branded email verification message for new user signups.
func RenderEmailVerification(recipientName, confirmURL string) (subject, htmlBody string) {
	name := recipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeURL := html.EscapeString(confirmURL)

	subject = "Verify your email to activate ScanDrix"
	preview := "Confirm your email address to activate your ScanDrix account and begin automated code reviews."

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>
			Welcome to <strong>ScanDrix</strong>! To activate your account and start reviewing pull requests with autonomous AI agents and AST security analysis, please confirm your email address.
		</p>

		<div style="background-color: #f8fafc; border-left: 4px solid #f8b76d; padding: 14px 18px; margin: 24px 0; font-size: 14px; color: #475569; border-radius: 0 6px 6px 0;">
			<strong>Security Notice:</strong> This verification link is cryptographically signed and expires in <strong>24 hours</strong>. If you did not create a ScanDrix account, you can safely disregard this email.
		</div>

		<p style="color: #64748b; font-size: 13px; margin-top: 24px; word-break: break-all;">
			If the button above does not work, copy and paste this URL into your browser:<br>
			<a href="%s" style="color: #2563eb; text-decoration: underline;">%s</a>
		</p>
	`, safeName, safeURL, safeURL)

	htmlBody = RenderBrandLayout(preview, "Activate Your ScanDrix Account", content, "Verify Email Address", confirmURL)
	return subject, htmlBody
}
