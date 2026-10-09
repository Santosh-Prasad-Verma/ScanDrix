package templates

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// PasswordResetDetails encapsulates security context for a password reset request.
type PasswordResetDetails struct {
	RecipientName  string
	ResetURL       string
	Device         string    // e.g. "macOS 15.3 · Chrome 124"
	IPAddress      string    // e.g. "198.51.100.42"
	Location       string    // e.g. "San Francisco, CA, US"
	RequestedAt    time.Time
	LockAccountURL string    // One-click URL to lock account / invalidate active sessions
}

// RenderPasswordReset generates a clean password reset email with default parameters.
func RenderPasswordReset(subscriberName, resetURL string) (subject, htmlBody string) {
	return RenderPasswordResetWithDetails(PasswordResetDetails{
		RecipientName: subscriberName,
		ResetURL:      resetURL,
	})
}

// RenderPasswordResetWithDetails generates a clean, uncluttered 15-minute expiring password reset email.
func RenderPasswordResetWithDetails(d PasswordResetDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Reset your ScanDrix Password"
	preview := "Reset your ScanDrix account password. This link is valid for 15 minutes."

	var detailsLine string
	if d.IPAddress != "" || d.Device != "" || d.Location != "" {
		parts := []string{}
		if d.Device != "" {
			parts = append(parts, html.EscapeString(d.Device))
		}
		if d.IPAddress != "" {
			parts = append(parts, html.EscapeString(d.IPAddress))
		}
		if d.Location != "" {
			parts = append(parts, html.EscapeString(d.Location))
		}
		detailsLine = fmt.Sprintf(`<p style="margin: 0 0 16px 0; font-size: 13px; color: #94a3b8;">Requested from %s.</p>`, strings.Join(parts, " · "))
	}

	lockAccountNote := "If you did not request this, you can safely ignore this email."
	if d.LockAccountURL != "" {
		lockAccountNote = fmt.Sprintf(`If you did not request this, please <a href="%s" style="color: #ef8557; text-decoration: underline;">lock your account immediately</a>.`, html.EscapeString(d.LockAccountURL))
	}

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">We received a request to reset your password. Click the button below to choose a new password. This link is valid for 15 minutes.</p>
		%s
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">%s</p>
	`, safeName, detailsLine, lockAccountNote)

	htmlBody = RenderBrandLayoutWithHero(preview, "Reset your ScanDrix password", content, "Reset Password", d.ResetURL, "", DrixyMascotThinking, "Drixy")
	return subject, htmlBody
}
