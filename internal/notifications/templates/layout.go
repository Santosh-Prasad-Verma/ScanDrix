package templates

import (
	"fmt"
	"html"
	"time"
)

// RenderBrandLayout renders a clean, minimalist white-and-black email shell.
func RenderBrandLayout(previewText, title, contentHTML, ctaText, ctaURL string) string {
	return RenderBrandLayoutWithFootnote(previewText, title, contentHTML, ctaText, ctaURL, "")
}

// RenderBrandLayoutWithFootnote renders the email shell with an optional subtle footnote below the CTA button.
func RenderBrandLayoutWithFootnote(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML string) string {
	var ctaBlock string
	safeTitle := html.EscapeString(title)
	safePreview := html.EscapeString(previewText)
	if ctaText != "" && ctaURL != "" {
		safeCtaText := html.EscapeString(ctaText)
		safeCtaURL := html.EscapeString(ctaURL)
		ctaBlock = fmt.Sprintf(`
			<div style="margin: 28px 0 20px 0;">
				<a href="%s" style="background-color: #000000; color: #ffffff; text-decoration: none; padding: 12px 28px; border-radius: 6px; font-weight: 600; font-size: 14px; display: inline-block; letter-spacing: -0.01em;">
					%s
				</a>
			</div>
		`, safeCtaURL, safeCtaText)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<meta name="color-scheme" content="light">
	<meta name="supported-color-schemes" content="light">
	<title>%s</title>
	<!--[if mso]>
	<noscript>
		<xml>
			<o:OfficeDocumentSettings>
				<o:PixelsPerInch>96</o:PixelsPerInch>
			</o:OfficeDocumentSettings>
		</xml>
	</noscript>
	<![endif]-->
	<style>
		body { margin: 0; padding: 0; background-color: %s; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; -webkit-font-smoothing: antialiased; }
		table { border-collapse: separate; }
		a { color: #000000; }
		@media only screen and (max-width: 620px) {
			.container { width: 100%% !important; padding: 16px 12px !important; }
			.card { padding: 32px 24px !important; }
		}
	</style>
</head>
<body style="background-color: %s; margin: 0; padding: 40px 0;">
	<!-- Hidden Preview Text -->
	<div style="display: none; max-height: 0px; overflow: hidden; font-size: 1px; line-height: 1px; color: %s;">
		%s
	</div>

	<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color: %s;">
		<tr>
			<td align="center">
				<div class="container" style="max-width: 560px; margin: 0 auto; width: 100%%; text-align: left;">
					<!-- Clean Minimalist Card -->
					<div class="card" style="background-color: %s; padding: 40px 40px; border: 1px solid %s; border-radius: 8px; box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);">
						<!-- Brand Logo -->
						<div style="margin-bottom: 32px;">
							<span style="font-size: 20px; font-weight: 800; color: #000000; letter-spacing: -0.03em;">
								ScanDrix
							</span>
						</div>

						<!-- Title -->
						<h1 style="color: %s; font-size: 20px; font-weight: 700; margin: 0 0 18px 0; line-height: 1.35; letter-spacing: -0.02em;">
							%s
						</h1>

						<!-- Body Content -->
						<div style="color: %s; font-size: 15px; line-height: 1.6;">
							%s
						</div>

						%s

						%s

						<!-- Divider -->
						<hr style="border: none; border-top: 1px solid #f3f4f6; margin: 32px 0 20px 0;">

						<!-- Footer Information -->
						<div style="color: %s; font-size: 12px; line-height: 1.5;">
							<p style="margin: 0 0 4px 0;">
								&copy; %d ScanDrix AI Inc. · Automated AI Code Reviews
							</p>
							<p style="margin: 0;">
								Questions? Contact <a href="mailto:support@scandrix.dev" style="color: %s; text-decoration: underline;">support@scandrix.dev</a>
							</p>
						</div>
					</div>
				</div>
			</td>
		</tr>
	</table>
</body>
</html>`,
		safeTitle,
		ColorPageBG,
		ColorPageBG,
		ColorPageBG,
		safePreview,
		ColorPageBG,
		ColorCardBG,
		ColorBorder,
		ColorTextPrimary,
		safeTitle,
		ColorTextPrimary,
		contentHTML,
		ctaBlock,
		footnoteHTML,
		ColorTextMuted,
		time.Now().Year(),
		ColorTextMuted,
	)
}
