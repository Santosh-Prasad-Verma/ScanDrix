package templates

import (
	"fmt"
	"html"
	"time"
)

// NewDeviceLoginDetails contains information about a sign-in attempt from a new or unrecognized source.
type NewDeviceLoginDetails struct {
	RecipientName  string
	Device         string    // e.g. "macOS 15.3 · Chrome 124"
	IPAddress      string    // e.g. "194.26.29.112"
	Location       string    // e.g. "Frankfurt, Germany"
	LoginTime      time.Time
	LockAccountURL string    // One-click URL to immediately invalidate all sessions and lock the account
	ActivityURL    string    // URL to view active sessions in the dashboard
}

// RenderNewDeviceLogin renders an alert email for unrecognized logins.
func RenderNewDeviceLogin(d NewDeviceLoginDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)

	locationStr := d.Location
	if locationStr == "" {
		locationStr = "an unrecognized location"
	}
	safeLocation := html.EscapeString(locationStr)

	subject = fmt.Sprintf("Security Alert: New login to ScanDrix from %s", safeLocation)
	preview := fmt.Sprintf("We detected a sign-in to your ScanDrix account from a new device or IP address (%s).", safeLocation)

	badge := RenderBadge("Security Alert", "#fef3c7", "#92400e", "#fde68a")

	rows := []InfoRow{
		{Label: "Device & Client", Value: html.EscapeString(d.Device)},
		{Label: "IP Address", Value: html.EscapeString(d.IPAddress)},
		{Label: "Approximate Location", Value: safeLocation},
	}
	if !d.LoginTime.IsZero() {
		rows = append(rows, InfoRow{Label: "Timestamp", Value: d.LoginTime.UTC().Format("Jan 02, 2006 · 15:04:05 MST")})
	}

	callout := `
		<strong>Was this you?</strong><br>
		If you recently logged in from this device or network, you can safely disregard this alert.<br><br>
		<strong>If this wasn't you</strong>, your password may have been exposed. Click below to instantly terminate all active sessions and protect your account.
	`
	calloutHTML := RenderCallout(callout, "#fca5a5", "#fef2f2", "#991b1b")

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">A new login was recently recorded for your ScanDrix account with the following details:</p>

		%s
		%s
	`, badge, safeName, RenderInfoTable(rows), calloutHTML)

	footnote := ""
	if d.ActivityURL != "" {
		footnote = fmt.Sprintf(`
			<p style="margin: 16px 0 0 0; font-size: 13px; color: #6b7280;">
				Want to inspect all signed-in devices? <a href="%s" style="color: #000000; font-weight: 600; text-decoration: underline;">Review Active Sessions</a>
			</p>
		`, html.EscapeString(d.ActivityURL))
	}

	htmlBody = RenderBrandLayoutWithFootnote(preview, "New device or location login", content, "Lock Account Immediately", d.LockAccountURL, footnote)
	return subject, htmlBody
}
