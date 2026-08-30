package templates

import (
	"fmt"
	"strings"
	"time"
)

// RenderBrandLayout renders the responsive HTML shell mirroring Kodus AI / ScanDrix brand design.
func RenderBrandLayout(previewText, title, contentHTML, ctaText, ctaURL string) string {
	var ctaBlock string
	if ctaText != "" && ctaURL != "" {
		ctaBlock = fmt.Sprintf(`
			<div style="text-align: center; margin: 32px 0 24px 0;">
				<a href="%s" style="background-color: %s; color: %s; text-decoration: none; padding: 14px 32px; border-radius: 6px; font-weight: 700; font-size: 15px; display: inline-block; letter-spacing: -0.01em; box-shadow: 0 2px 4px rgba(0,0,0,0.06);">
					%s &rarr;
				</a>
			</div>
		`, ctaURL, ColorPrimaryLight, ColorPrimaryDark, ctaText)
	}

	previewEscaped := strings.ReplaceAll(previewText, "\"", "&quot;")

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
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
		body { margin: 0; padding: 0; background-color: %s; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Oxygen, Ubuntu, Cantarell, "Fira Sans", "Droid Sans", "Helvetica Neue", sans-serif; -webkit-font-smoothing: antialiased; }
		table { border-collapse: separate; }
		a { color: %s; }
		@media only screen and (max-width: 620px) {
			.container { width: 100%% !important; padding: 12px !important; }
			.card { padding: 24px 20px !important; }
			.header { padding: 16px 20px !important; }
		}
	</style>
</head>
<body style="background-color: %s; margin: 0; padding: 24px 0;">
	<!-- Hidden Preview Text -->
	<div style="display: none; max-height: 0px; overflow: hidden;">
		%s
	</div>

	<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%">
		<tr>
			<td align="center">
				<div class="container" style="max-width: 580px; margin: 0 auto; width: 100%%;">
					<!-- Dark Brand Header Banner -->
					<div class="header" style="background-color: %s; padding: 20px 40px; border-radius: 8px 8px 0 0; text-align: left;">
						<table border="0" cellpadding="0" cellspacing="0" width="100%%">
							<tr>
								<td>
									<span style="font-size: 20px; font-weight: 800; color: #ffffff; letter-spacing: -0.03em;">
										Scan<span style="color: %s;">Drix</span>
									</span>
									<span style="display: inline-block; margin-left: 8px; font-size: 11px; text-transform: uppercase; font-weight: 700; background-color: rgba(248, 183, 109, 0.15); color: %s; padding: 2px 8px; border-radius: 4px; border: 1px solid rgba(248, 183, 109, 0.3);">
										AI Code Review
									</span>
								</td>
							</tr>
						</table>
					</div>

					<!-- Card Body -->
					<div class="card" style="background-color: %s; padding: 36px 40px; border: 1px solid %s; border-top: none; border-radius: 0 0 8px 8px; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05);">
						<h1 style="color: %s; font-size: 22px; font-weight: 700; margin: 0 0 20px 0; line-height: 1.3; letter-spacing: -0.02em;">
							%s
						</h1>

						<div style="color: %s; font-size: 15px; line-height: 1.6;">
							%s
						</div>

						%s

						<hr style="border: none; border-top: 1px solid %s; margin: 32px 0 20px 0;">

						<!-- Footer Information -->
						<div style="color: %s; font-size: 12px; line-height: 1.6; text-align: center;">
							<p style="margin: 0 0 8px 0;">
								&copy; %d ScanDrix AI Inc. All rights reserved.
							</p>
							<p style="margin: 0; color: #9ca3af;">
								Autonomous AI Code Review &amp; Security Engineering Platform<br>
								Need help? Contact our enterprise support team at <a href="mailto:support@scandrix.dev" style="color: #6b7280; text-decoration: underline;">support@scandrix.dev</a>
							</p>
						</div>
					</div>
				</div>
			</td>
		</tr>
	</table>
</body>
</html>`,
		title,
		ColorPageBG,
		ColorPrimaryDark,
		ColorPageBG,
		previewEscaped,
		ColorHeaderBG,
		ColorPrimaryLight,
		ColorPrimaryLight,
		ColorCardBG,
		ColorBorder,
		ColorTextPrimary,
		title,
		ColorTextPrimary,
		contentHTML,
		ctaBlock,
		ColorBorder,
		ColorTextMuted,
		time.Now().Year(),
	)
}
