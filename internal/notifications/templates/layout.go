package templates

import (
	"fmt"
	"html"
	"time"
)

// RenderBrandLayout renders the email shell using the ambient gradients style.
func RenderBrandLayout(previewText, title, contentHTML, ctaText, ctaURL string) string {
	return RenderBrandLayoutWithHero(previewText, title, contentHTML, ctaText, ctaURL, "", "", "")
}

// RenderBrandLayoutWithFootnote renders the email shell with an optional subtle footnote below the CTA button.
func RenderBrandLayoutWithFootnote(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML string) string {
	return RenderBrandLayoutWithHero(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML, "", "")
}

// RenderBrandLayoutWithHero renders the email shell with an ambient header, optional Drixy mascot hero, and deep dark indigo palette.
func RenderBrandLayoutWithHero(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML, drixyImage, drixyAlt string) string {
	var ctaBlock string
	safeTitle := html.EscapeString(title)
	safePreview := html.EscapeString(previewText)
	if ctaText != "" && ctaURL != "" {
		safeCtaText := html.EscapeString(ctaText)
		safeCtaURL := html.EscapeString(ctaURL)
		ctaBlock = fmt.Sprintf(`
			<div style="margin: 28px 0 20px 0;">
				<a href="%s" style="background: linear-gradient(135deg, #4338ca 0%%, #6366f1 100%%); background-color: #4f46e5; color: #ffffff; text-decoration: none; padding: 13px 30px; border-radius: 8px; font-weight: 600; font-size: 14px; display: inline-block; letter-spacing: -0.01em; box-shadow: 0 4px 14px rgba(79, 70, 229, 0.45);">
					%s
				</a>
			</div>
		`, safeCtaURL, safeCtaText)
	}

	var mascotHeroHTML string
	if drixyImage != "" {
		mascotHeroHTML = RenderDrixyMascot(drixyImage, drixyAlt)
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
		body { margin: 0; padding: 0; background-color: #060710; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; -webkit-font-smoothing: antialiased; }
		table { border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; }
		a { color: #6366f1; text-decoration: none; }
		@media only screen and (max-width: 620px) {
			.container { width: 100%% !important; padding: 16px 10px !important; }
			.card-body { padding: 24px 20px !important; }
			.card-header { padding: 24px 20px 16px 20px !important; }
		}
	</style>
</head>
<body style="background-color: #060710; margin: 0; padding: 36px 0;">
	<!-- Hidden Preview Text -->
	<div style="display: none; max-height: 0px; overflow: hidden; font-size: 1px; line-height: 1px; color: #060710;">
		%s
	</div>

	<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color: #060710;">
		<tr>
			<td align="center">
				<div class="container" style="max-width: 580px; margin: 0 auto; width: 100%%; text-align: left;">
					<!-- Ambient Gradients Card Container -->
					<div style="background-color: #0d1322; border: 1px solid #1e293b; border-radius: 16px; overflow: hidden; box-shadow: 0 20px 50px -15px rgba(0, 0, 0, 0.7);">
						
						<!-- Ambient Header Hero Gradient -->
						<div class="card-header" style="background: #0d1527; background-image: linear-gradient(180deg, #131d36 0%%, #0d1527 60%%, #0d1322 100%%); padding: 32px 36px 18px 36px; border-bottom: 1px solid rgba(30, 41, 59, 0.6);">
							<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%">
								<tr>
									<td style="vertical-align: middle;">
										<span style="font-size: 21px; font-weight: 800; color: #ffffff; letter-spacing: -0.03em;">
											ScanDrix
										</span>
										<span style="display: inline-block; background-color: rgba(99, 102, 241, 0.15); color: #c7d2fe; border: 1px solid rgba(99, 102, 241, 0.35); font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em; padding: 2px 8px; border-radius: 10px; margin-left: 8px; vertical-align: middle;">
											AI Code Review
										</span>
									</td>
								</tr>
							</table>

							<!-- Drixy Mascot Hero (Contextual) -->
							%s
						</div>

						<!-- Card Body Content -->
						<div class="card-body" style="padding: 28px 36px 36px 36px;">
							<!-- Title -->
							<h1 style="color: #ffffff; font-size: 20px; font-weight: 700; margin: 0 0 16px 0; line-height: 1.35; letter-spacing: -0.02em;">
								%s
							</h1>

							<!-- Content Body -->
							<div style="color: #cbd5e1; font-size: 15px; line-height: 1.65;">
								%s
							</div>

							%s

							%s
						</div>

						<!-- Divider and Dark Footer -->
						<div style="border-top: 1px solid #1e293b; background-color: #080c16; padding: 22px 36px; text-align: center;">
							<div style="color: #64748b; font-size: 12px; line-height: 1.55;">
								<p style="margin: 0 0 4px 0;">
									&copy; %d ScanDrix AI Inc. · Automated AI Code Reviews
								</p>
								<p style="margin: 0;">
									Need assistance? Contact <a href="mailto:support@scandrix.dev" style="color: #818cf8; text-decoration: underline;">support@scandrix.dev</a>
								</p>
							</div>
						</div>
					</div>
				</div>
			</td>
		</tr>
	</table>
</body>
</html>`,
		safeTitle,
		safePreview,
		mascotHeroHTML,
		safeTitle,
		contentHTML,
		ctaBlock,
		footnoteHTML,
		time.Now().Year(),
	)
}
