package templates

import (
	"fmt"
	"html"
	"strings"
)

// InfoRow represents a labeled property row in an email summary card.
type InfoRow struct {
	Label string
	Value string
}

// StatItem represents a prominent key metric in an email dashboard card.
type StatItem struct {
	Value string
	Label string
}

// RenderBadge renders a subtle rounded badge pill.
func RenderBadge(text, bgHex, textHex, borderHex string) string {
	safeText := html.EscapeString(text)
	return fmt.Sprintf(`
		<span style="display: inline-block; font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em; padding: 4px 10px; border-radius: 4px; background-color: %s; color: %s; border: 1px solid %s; margin-bottom: 14px;">
			%s
		</span>
	`, bgHex, textHex, borderHex, safeText)
}

// RenderCallout renders an alert callout with a thick left accent border.
func RenderCallout(contentHTML, borderHex, bgHex, textHex string) string {
	return fmt.Sprintf(`
		<div style="background-color: %s; border: 1px solid %s; border-left: 3px solid %s; border-radius: 6px; padding: 14px 18px; margin: 20px 0; font-size: 14px; color: %s; line-height: 1.55;">
			%s
		</div>
	`, bgHex, borderHex, borderHex, textHex, contentHTML)
}

// RenderInfoTable renders a clean, tabular metadata card.
func RenderInfoTable(rows []InfoRow) string {
	if len(rows) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; margin: 20px 0; border-collapse: separate; overflow: hidden;">`)

	for i, r := range rows {
		borderBottom := "border-bottom: 1px solid #f3f4f6;"
		if i == len(rows)-1 {
			borderBottom = ""
		}
		sb.WriteString(fmt.Sprintf(`
			<tr>
				<td style="padding: 10px 16px; width: 34%%; font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; color: #6b7280; %s">
					%s
				</td>
				<td style="padding: 10px 16px; font-size: 13px; font-weight: 500; color: #111827; %s">
					%s
				</td>
			</tr>
		`, borderBottom, html.EscapeString(r.Label), borderBottom, r.Value))
	}

	sb.WriteString(`</table>`)
	return sb.String()
}

// RenderStatGrid renders a 2-column or 4-column metric grid.
func RenderStatGrid(stats []StatItem) string {
	if len(stats) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="margin: 20px 0; border-collapse: separate;">`)

	for i := 0; i < len(stats); i += 2 {
		sb.WriteString(`<tr>`)

		// Left item
		sb.WriteString(fmt.Sprintf(`
			<td style="width: 48%%; vertical-align: top; padding: 0 4px 10px 0;">
				<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 14px 12px; text-align: center;">
					<div style="font-size: 22px; font-weight: 800; color: #111827; line-height: 1.2; letter-spacing: -0.02em;">%s</div>
					<div style="font-size: 12px; font-weight: 500; color: #6b7280; margin-top: 4px;">%s</div>
				</div>
			</td>
		`, html.EscapeString(stats[i].Value), html.EscapeString(stats[i].Label)))

		// Right item (if exists)
		if i+1 < len(stats) {
			sb.WriteString(fmt.Sprintf(`
				<td style="width: 48%%; vertical-align: top; padding: 0 0 10px 4px;">
					<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 14px 12px; text-align: center;">
						<div style="font-size: 22px; font-weight: 800; color: #111827; line-height: 1.2; letter-spacing: -0.02em;">%s</div>
						<div style="font-size: 12px; font-weight: 500; color: #6b7280; margin-top: 4px;">%s</div>
					</div>
				</td>
			`, html.EscapeString(stats[i+1].Value), html.EscapeString(stats[i+1].Label)))
		} else {
			sb.WriteString(`<td style="width: 48%%; padding: 0 0 10px 4px;"></td>`)
		}

		sb.WriteString(`</tr>`)
	}

	sb.WriteString(`</table>`)
	return sb.String()
}

// RenderProgressBar renders an email-safe table progress bar.
func RenderProgressBar(percent int, fillHex string) string {
	if percent < 0 {
		percent = 0
	}
	fillPercent := percent
	if fillPercent > 100 {
		fillPercent = 100
	}
	remaining := 100 - fillPercent

	return fmt.Sprintf(`
		<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="height: 10px; background-color: #e5e7eb; border-radius: 5px; overflow: hidden; margin: 12px 0 6px 0;">
			<tr>
				<td width="%d%%" style="background-color: %s; height: 10px; border-radius: 5px 0 0 5px;"></td>
				<td width="%d%%" style="height: 10px;"></td>
			</tr>
		</table>
	`, fillPercent, fillHex, remaining)
}

// RenderCodeSnippet renders a monospace code box.
func RenderCodeSnippet(code string) string {
	safeCode := html.EscapeString(code)
	return fmt.Sprintf(`
		<div style="background-color: #f3f4f6; border: 1px solid #e5e7eb; border-radius: 6px; padding: 10px 14px; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 13px; color: #111827; word-break: break-all; margin: 14px 0;">
			%s
		</div>
	`, safeCode)
}
