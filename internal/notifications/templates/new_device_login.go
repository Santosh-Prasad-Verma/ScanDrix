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

// RenderNewDeviceLogin renders a clean alert email for unrecognized logins without loud highlight boxes.
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
	preview := fmt.Sprintf("We detected a sign-in to your ScanDrix account from %s.", safeLocation)

	deviceInfo := html.EscapeString(d.Device)
	if deviceInfo == "" {
		deviceInfo = "Unknown device"
	}
	ipInfo := html.EscapeString(d.IPAddress)

	var sessionsLink string
	if d.ActivityURL != "" {
		sessionsLink = fmt.Sprintf(`<p style="margin: 16px 0 0 0; font-size: 13px; color: #94a3b8;">Want to inspect all signed-in devices? <a href="%s" style="color: #ef8557; text-decoration: underline;">Review Active Sessions</a></p>`, html.EscapeString(d.ActivityURL))
	}

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">We detected a new sign-in to your ScanDrix account:</p>
		<p style="margin: 0 0 16px 0; font-size: 14px; color: #ffffff; line-height: 1.6;">
			<strong>Device:</strong> %s<br>
			<strong>Location:</strong> %s<br>
			<strong>IP Address:</strong> %s
		</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">If this was you, you can safely ignore this email. If not, lock your account immediately to terminate all active sessions.</p>
		%s
	`, safeName, deviceInfo, safeLocation, ipInfo, sessionsLink)

	htmlBody = RenderBrandLayoutWithHero(preview, "New device login detected", content, "Lock Account Immediately", d.LockAccountURL, "", DrixyMascotStandingReady, "Drixy Device Security")
	return subject, htmlBody
}
