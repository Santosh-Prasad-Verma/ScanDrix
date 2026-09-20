// Package email provides transactional email templates, Resend/SMTP delivery clients, and brand styling for ScanDrix.
package email

import (
	"fmt"
)

// EmailFrom specifies the system sender personality.
type EmailFrom string

const (
	FromNoReply       EmailFrom = "noreply"
	FromNotifications EmailFrom = "notifications"
	FromSupport       EmailFrom = "support"
	FromSecurity      EmailFrom = "security"
	FromBilling       EmailFrom = "billing"
	FromAlerts        EmailFrom = "alerts"
	FromReports       EmailFrom = "reports"
)

// FormatFromAddress formats the sender address using scandrix.dev domain.
func FormatFromAddress(from EmailFrom) string {
	switch from {
	case FromNotifications:
		return "Drixy from ScanDrix <notifications@scandrix.dev>"
	case FromSupport:
		return "ScanDrix Support <support@scandrix.dev>"
	case FromSecurity:
		return "ScanDrix Security <security@scandrix.dev>"
	case FromBilling:
		return "ScanDrix Billing <billing@scandrix.dev>"
	case FromAlerts:
		return "ScanDrix Alerts <alerts@scandrix.dev>"
	case FromReports:
		return "ScanDrix Reports <reports@scandrix.dev>"
	case FromNoReply:
		fallthrough
	default:
		return "ScanDrix <noreply@scandrix.dev>"
	}
}

// BrandColors defines styling tokens matching the ScanDrix design system.
const (
	ColorPrimaryLight = "#f8b76d"
	ColorPrimaryDark  = "#443024"
	ColorBackground   = "#101019"
	ColorTextColor    = "#1F2937"
	ColorMutedText    = "#6B7280"
	ColorCardBg       = "#FFFFFF"
	ColorPageBg       = "#F4F4F5"
	ColorDivider      = "#E5E7EB"
	DomainName        = "scandrix.dev"
	BaseWebURL        = "https://app.scandrix.dev"
)

// RenderBrandLayout wraps email body content in a responsive HTML layout.
func RenderBrandLayout(previewText, contentHTML string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ScanDrix</title>
<style>
body { background-color: %s; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 24px 0; color: %s; }
.container { max-width: 560px; margin: 0 auto; border-radius: 8px; overflow: hidden; background: %s; border: 1px solid %s; }
.header { background-color: %s; padding: 24px 32px; text-align: left; }
.header h1 { color: #FFFFFF; font-size: 20px; margin: 0; font-weight: 700; letter-spacing: -0.5px; }
.header h1 span { color: %s; }
.body { padding: 32px; line-height: 1.6; font-size: 15px; }
.btn { display: inline-block; background-color: %s; color: %s !important; padding: 12px 24px; border-radius: 6px; font-weight: 600; text-decoration: none; margin: 20px 0; font-size: 14px; }
.footer { border-top: 1px solid %s; padding: 20px 32px; text-align: center; font-size: 12px; color: %s; }
.footer a { color: %s; text-decoration: underline; }
.preview-hidden { display: none; font-size: 0px; max-height: 0px; overflow: hidden; }
</style>
</head>
<body>
<div class="preview-hidden">%s</div>
<div class="container">
  <div class="header">
    <h1>Scan<span>Drix</span></h1>
  </div>
  <div class="body">
    %s
  </div>
  <div class="footer">
    ScanDrix Inc. · <a href="https://%s">%s</a> · <a href="mailto:support@%s">support@%s</a>
  </div>
</div>
</body>
</html>`,
		ColorPageBg, ColorTextColor, ColorCardBg, ColorDivider,
		ColorBackground, ColorPrimaryLight,
		ColorPrimaryLight, ColorPrimaryDark,
		ColorDivider, ColorMutedText, ColorMutedText,
		previewText, contentHTML,
		DomainName, DomainName, DomainName, DomainName)
}
