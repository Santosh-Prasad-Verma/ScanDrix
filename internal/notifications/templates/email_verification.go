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

		<div style="background-color: #16171b; border: 1px solid #26282f; border-left: 4px solid #c9f36b; padding: 16px 20px; margin: 24px 0; font-size: 14px; color: #d4d4d8; border-radius: 0 8px 8px 0;">
			<strong style="color: #ffffff;">Security Notice:</strong> This verification link is cryptographically signed and expires in <strong>24 hours</strong>. If you did not create a ScanDrix account, you can safely disregard this email.
		</div>

		<p style="color: #71717a; font-size: 13px; margin-top: 24px; word-break: break-all;">
			If the button above does not work, copy and paste this URL into your browser:<br>
			<a href="%s" style="color: #c9f36b; text-decoration: underline;">%s</a>
		</p>
	`, safeName, safeURL, safeURL)

	htmlBody = RenderBrandLayout(preview, "Activate Your ScanDrix Account", content, "Verify Email Address", confirmURL)
	return subject, htmlBody
}
