package templates

import (
	"fmt"
	"html"
	"time"
)

// RenderBrandLayout renders the responsive HTML shell with the ScanDrix brand design.
func RenderBrandLayout(previewText, title, contentHTML, ctaText, ctaURL string) string {
	var ctaBlock string
	safeTitle := html.EscapeString(title)
	safePreview := html.EscapeString(previewText)
	if ctaText != "" && ctaURL != "" {
		safeCtaText := html.EscapeString(ctaText)
		safeCtaURL := html.EscapeString(ctaURL)
		ctaBlock = fmt.Sprintf(`
			<div style="text-align: center; margin: 32px 0 24px 0;">
				<a href="%s" style="background-color: %s; color: %s; text-decoration: none; padding: 14px 34px; border-radius: 8px; font-weight: 700; font-size: 15px; display: inline-block; letter-spacing: -0.01em; box-shadow: 0 0 24px rgba(201, 243, 107, 0.25);">
					%s &rarr;
				</a>
			</div>
		`, safeCtaURL, ColorPrimaryLight, ColorPrimaryDark, safeCtaText)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<meta name="color-scheme" content="dark">
	<meta name="supported-color-schemes" content="dark">
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
		a { color: %s; }
		@media only screen and (max-width: 620px) {
			.container { width: 100%% !important; padding: 12px 8px !important; }
			.card { padding: 28px 20px !important; }
			.header { padding: 20px 20px !important; }
		}
	</style>
</head>
<body style="background-color: %s; margin: 0; padding: 32px 0;">
	<!-- Hidden Preview Text -->
	<div style="display: none; max-height: 0px; overflow: hidden; font-size: 1px; line-height: 1px; color: %s;">
		%s
	</div>

	<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color: %s;">
		<tr>
			<td align="center">
				<div class="container" style="max-width: 580px; margin: 0 auto; width: 100%%;">
					<!-- Dark Brand Header Banner -->
					<div class="header" style="background-color: %s; padding: 22px 36px; border-radius: 12px 12px 0 0; border: 1px solid %s; border-bottom: 1px solid #1a1b1e; text-align: left;">
						<table border="0" cellpadding="0" cellspacing="0" width="100%%">
							<tr>
								<td>
									<span style="font-size: 21px; font-weight: 800; color: #ffffff; letter-spacing: -0.02em;">
										Scan<span style="color: %s;">Drix</span>
									</span>
									<span style="display: inline-block; margin-left: 10px; font-size: 11px; text-transform: uppercase; font-weight: 700; background-color: rgba(201, 243, 107, 0.12); color: %s; padding: 3px 8px; border-radius: 4px; border: 1px solid rgba(201, 243, 107, 0.3); letter-spacing: 0.04em;">
										AI Code Review
									</span>
								</td>
							</tr>
						</table>
					</div>

					<!-- Card Body -->
					<div class="card" style="background-color: %s; padding: 36px 36px; border: 1px solid %s; border-top: none; border-radius: 0 0 12px 12px; box-shadow: 0 8px 30px rgba(0, 0, 0, 0.6);">
						<h1 style="color: %s; font-size: 22px; font-weight: 700; margin: 0 0 18px 0; line-height: 1.3; letter-spacing: -0.02em;">
							%s
						</h1>

						<div style="color: #e4e4e7; font-size: 15px; line-height: 1.65;">
							%s
						</div>

						%s

						<hr style="border: none; border-top: 1px solid %s; margin: 32px 0 20px 0;">

						<!-- Footer Information -->
						<div style="color: %s; font-size: 12px; line-height: 1.6; text-align: center;">
							<p style="margin: 0 0 6px 0; color: #a1a1aa;">
								&copy; %d ScanDrix AI Inc. All rights reserved.
							</p>
							<p style="margin: 0; color: %s;">
								Autonomous AI Code Review &amp; Deterministic AST Security Platform<br>
								Questions? Contact our team at <a href="mailto:support@scandrix.dev" style="color: %s; text-decoration: underline;">support@scandrix.dev</a>
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
		ColorPrimaryLight,
		ColorPageBG,
		ColorPageBG,
		safePreview,
		ColorPageBG,
		ColorHeaderBG,
		ColorBorder,
		ColorPrimaryLight,
		ColorPrimaryLight,
		ColorCardBG,
		ColorBorder,
		ColorTextPrimary,
		safeTitle,
		contentHTML,
		ctaBlock,
		ColorBorder,
		ColorTextMuted,
		time.Now().Year(),
		ColorTextMuted,
		ColorPrimaryLight,
	)
}
