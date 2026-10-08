package templates

import (
	"fmt"
	"html"
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

// RenderPasswordReset generates a clean white-and-black password reset email with default parameters.
func RenderPasswordReset(subscriberName, resetURL string) (subject, htmlBody string) {
	return RenderPasswordResetWithDetails(PasswordResetDetails{
		RecipientName: subscriberName,
		ResetURL:      resetURL,
	})
}

// RenderPasswordResetWithDetails generates a high-security, 15-minute expiring password reset email.
func RenderPasswordResetWithDetails(d PasswordResetDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	subject = "Reset your ScanDrix Password"
	preview := "A password reset request was received for your ScanDrix account. This link expires in 15 minutes."

	badge := RenderBadge("Security Notice", "#f3f4f6", "#111827", "#e5e7eb")

	// Build request context table if metadata is available
	var infoRows []InfoRow
	if d.Device != "" {
		infoRows = append(infoRows, InfoRow{Label: "Device & Browser", Value: html.EscapeString(d.Device)})
	}
	if d.IPAddress != "" {
		infoRows = append(infoRows, InfoRow{Label: "IP Address", Value: html.EscapeString(d.IPAddress)})
	}
	if d.Location != "" {
		infoRows = append(infoRows, InfoRow{Label: "Location", Value: html.EscapeString(d.Location)})
	}
	if !d.RequestedAt.IsZero() {
		infoRows = append(infoRows, InfoRow{Label: "Requested At", Value: d.RequestedAt.UTC().Format("Jan 02, 2006 · 15:04 MST")})
	}

	metadataHTML := ""
	if len(infoRows) > 0 {
		metadataHTML = fmt.Sprintf(`
			<p style="margin: 20px 0 6px 0; font-size: 13px; font-weight: 600; color: #374151;">
				Request Details:
			</p>
			%s
		`, RenderInfoTable(infoRows))
	}

	// Security warning / lock account callout
	var warningHTML string
	if d.LockAccountURL != "" {
		safeLockURL := html.EscapeString(d.LockAccountURL)
		calloutMsg := fmt.Sprintf(`<strong>Didn't make this request?</strong> If you did not request a password reset, someone may be attempting to access your account. Please <a href="%s" style="color: #991b1b; font-weight: 600; text-decoration: underline;">lock your account immediately</a> to terminate active sessions.`, safeLockURL)
		warningHTML = RenderCallout(calloutMsg, "#fca5a5", "#fef2f2", "#991b1b")
	} else {
		calloutMsg := `<strong>Didn't make this request?</strong> If you did not request a password reset, you can safely ignore this email. Your password will remain unchanged.`
		warningHTML = RenderCallout(calloutMsg, "#e5e7eb", "#f9fafb", "#4b5563")
	}

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0;">We received a request to reset the password for your ScanDrix account. Click the button below to set a new password:</p>

		<div style="background-color: #fffbeb; border: 1px solid #fef3c7; border-radius: 6px; padding: 10px 14px; margin: 18px 0; font-size: 13px; color: #92400e;">
			⏳ <strong>Note:</strong> This password reset link is valid for <strong>15 minutes</strong>.
		</div>

		%s
		%s
	`, badge, safeName, metadataHTML, warningHTML)

	footnote := fmt.Sprintf(`
		<p style="margin: 16px 0 0 0; font-size: 12px; color: #9ca3af; word-break: break-all;">
			If the button does not work, copy and paste this URL into your browser:<br>
			<span style="color: #4b5563;">%s</span>
		</p>
	`, html.EscapeString(d.ResetURL))

	htmlBody = RenderBrandLayoutWithFootnote(preview, "Reset your ScanDrix password", content, "Reset Password", d.ResetURL, footnote)
	return subject, htmlBody
}
