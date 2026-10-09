package templates

import (
	"fmt"
	"html"
	"time"
)

// RenderBrandLayout renders the email shell using the general banner.
func RenderBrandLayout(previewText, title, contentHTML, ctaText, ctaURL string) string {
	return RenderBrandLayoutWithBanner(previewText, title, contentHTML, ctaText, ctaURL, "", "", "", BannerGeneral)
}

// RenderBrandLayoutWithFootnote renders the email shell with an optional subtle footnote below the CTA button.
func RenderBrandLayoutWithFootnote(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML string) string {
	return RenderBrandLayoutWithBanner(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML, "", "", BannerGeneral)
}

// RenderBrandLayoutWithHero renders the email shell with the general banner, optional Drixy mascot hero, and noise theme.
func RenderBrandLayoutWithHero(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML, drixyImage, drixyAlt string) string {
	return RenderBrandLayoutWithBanner(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML, drixyImage, drixyAlt, BannerGeneral)
}

// RenderBrandLayoutWithBanner renders the full email shell with a customizable header banner, noise texture background, and footer logo mark.
func RenderBrandLayoutWithBanner(previewText, title, contentHTML, ctaText, ctaURL, footnoteHTML, drixyImage, drixyAlt, bannerImage string) string {
	var ctaBlock string
	safeTitle := html.EscapeString(title)
	safePreview := html.EscapeString(previewText)
	if ctaText != "" && ctaURL != "" {
		safeCtaText := html.EscapeString(ctaText)
		safeCtaURL := html.EscapeString(ctaURL)
		ctaBlock = fmt.Sprintf(`
			<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin: 28px 0 20px 0;">
				<tr>
					<td align="left" style="border-radius: 6px; background-color: #ef8557;">
						<a href="%s" target="_blank" style="background-color: #ef8557; color: #ffffff; text-decoration: none; padding: 13px 28px; border-radius: 6px; font-weight: 600; font-size: 14px; display: inline-block; letter-spacing: -0.01em; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
							%s
						</a>
					</td>
				</tr>
			</table>
		`, safeCtaURL, safeCtaText)
	}

	var mascotHeroHTML string
	if drixyImage != "" {
		mascotHeroHTML = RenderDrixyMascot(drixyImage, drixyAlt)
	}

	if bannerImage == "" {
		bannerImage = BannerGeneral
	}
	bannerURL := GetAssetURL(bannerImage)
	footerLogoURL := GetAssetURL(FooterLogo)

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
		body { 
			margin: 0; padding: 0; 
			background-color: #060812; 
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; 
			-webkit-font-smoothing: antialiased; 
			-webkit-text-size-adjust: 100%%;
			-ms-text-size-adjust: 100%%;
		}
		table { border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; }
		a { color: #f97316; text-decoration: none; }
		@media only screen and (max-width: 620px) {
			.card-table { width: 100%% !important; max-width: 100%% !important; }
			.card-body { padding: 24px 20px !important; }
			.card-footer { padding: 22px 20px !important; }
			.banner-img { width: 100%% !important; height: auto !important; }
		}
	</style>
</head>
<body style="background-color: #060812; margin: 0; padding: 0; width: 100%%; -webkit-text-size-adjust: 100%%; -ms-text-size-adjust: 100%%;">
	<!-- Hidden Preview Text -->
	<div style="display: none; max-height: 0px; overflow: hidden; font-size: 1px; line-height: 1px; color: #060812;">
		%s
	</div>

	<!-- Outer Canvas Wrapper -->
	<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" bgcolor="#060812" style="background-color: #060812; width: 100%%; margin: 0; padding: 0; border-collapse: collapse;">
		<tr>
			<td align="center" valign="top" style="padding: 32px 12px; background-color: #060812;">
				<!--[if (gte mso 9)|(IE)]>
				<table align="center" border="0" cellpadding="0" cellspacing="0" width="580" style="width: 580px;">
					<tr>
						<td align="center" valign="top">
				<![endif]-->

				<!-- Main Card Table -->
				<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" class="card-table" style="max-width: 580px; width: 100%%; margin: 0 auto; background-color: #0a0e1c; border: 1px solid #1e293b; border-top: 2px solid #ef8557; border-radius: 12px; border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; box-shadow: 0 10px 30px rgba(0, 0, 0, 0.6);">
					<!-- Top Hero Banner Image -->
					<tr>
						<td align="center" style="padding: 0; margin: 0; background-color: #0b1a42; border-bottom: 1px solid #1e293b; line-height: 0; font-size: 0; border-top-left-radius: 10px; border-top-right-radius: 10px;">
							<img src="%s" alt="ScanDrix" width="580" border="0" class="banner-img" style="display: block; width: 100%%; max-width: 580px; height: auto; border: 0; border-top-left-radius: 10px; border-top-right-radius: 10px; outline: none; text-decoration: none;" />
						</td>
					</tr>

					<!-- Card Body Content -->
					<tr>
						<td class="card-body" style="padding: 32px 32px 36px 32px; background-color: #0a0e1c; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; text-align: left;">
							<!-- Drixy Mascot (Contextual) -->
							%s

							<!-- Title -->
							<h1 style="color: #ffffff; font-size: 21px; font-weight: 700; margin: 0 0 16px 0; line-height: 1.35; letter-spacing: -0.02em; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
								%s
							</h1>

							<!-- Content Body -->
							<div style="color: #cbd5e1; font-size: 15px; line-height: 1.65; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
								%s
							</div>

							%s

							%s
						</td>
					</tr>

					<!-- Divider and Dark Footer with Brand Logo Mark -->
					<tr>
						<td class="card-footer" align="center" style="padding: 24px 32px 28px 32px; background-color: #060913; border-top: 1px solid #1e293b; border-bottom-left-radius: 10px; border-bottom-right-radius: 10px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; text-align: center;">
							<div style="margin-bottom: 14px;">
								<img src="%s" alt="ScanDrix" width="44" height="42" border="0" style="width: 44px; height: 42px; display: inline-block; margin: 0 auto; border: 0; outline: none;" />
							</div>
							<p style="margin: 0 0 6px 0; color: #94a3b8; font-size: 12px; font-weight: 500; line-height: 1.5; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
								&copy; %d ScanDrix AI Inc. &middot; Automated AI Code Reviews
							</p>
							<p style="margin: 0; color: #64748b; font-size: 12px; line-height: 1.5; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
								Need assistance? Contact <a href="mailto:support@scandrix.dev" style="color: #ef8557; font-weight: 600; text-decoration: underline;">support@scandrix.dev</a>
							</p>
						</td>
					</tr>
				</table>

				<!--[if (gte mso 9)|(IE)]>
						</td>
					</tr>
				</table>
				<![endif]-->
			</td>
		</tr>
	</table>
</body>
</html>`,
		safeTitle,
		safePreview,
		bannerURL,
		mascotHeroHTML,
		safeTitle,
		contentHTML,
		ctaBlock,
		footnoteHTML,
		footerLogoURL,
		time.Now().Year(),
	)
}
