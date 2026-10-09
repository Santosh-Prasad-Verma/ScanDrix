package templates

import (
	"fmt"
	"html"
)

// RenderInvoiceReceipt generates the itemized payment receipt and invoice email with ambient gradient styling.
func RenderInvoiceReceipt(inv InvoiceDetails) (subject, htmlBody string) {
	safeInvoiceNumber := html.EscapeString(inv.InvoiceNumber)
	safeAmount := html.EscapeString(inv.AmountFormatted)
	safeRecipientName := html.EscapeString(inv.RecipientName)
	safePaymentID := html.EscapeString(inv.PaymentID)
	safePlanTier := html.EscapeString(inv.PlanTier)

	subject = fmt.Sprintf("Your ScanDrix Invoice %s (%s)", safeInvoiceNumber, safeAmount)
	preview := fmt.Sprintf("Thank you for your payment. ScanDrix invoice %s for %s has been processed.", safeInvoiceNumber, safeAmount)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 20px 0;">Thank you for your payment. We have successfully processed your invoice for the <strong>%s</strong> plan.</p>

		<div style="background-color: #111827; border: 1px solid #1e293b; border-radius: 8px; padding: 20px 22px; margin: 24px 0;">
			<table border="0" cellpadding="6" cellspacing="0" width="100%%" style="font-size: 14px; color: #cbd5e1;">
				<tr>
					<td style="color: #94a3b8; width: 40%%;">Invoice:</td>
					<td style="font-weight: 600; color: #ffffff;">%s</td>
				</tr>
				<tr>
					<td style="color: #94a3b8;">Amount Paid:</td>
					<td style="font-weight: 700; color: #ffffff;">%s <span style="display: inline-block; background-color: rgba(16, 185, 129, 0.15); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.4); font-size: 11px; font-weight: 700; padding: 2px 6px; border-radius: 4px; margin-left: 6px;">PAID</span></td>
				</tr>
				<tr>
					<td style="color: #94a3b8;">Payment ID:</td>
					<td style="font-family: monospace; font-size: 13px; color: #38bdf8;">%s</td>
				</tr>
				<tr>
					<td style="color: #94a3b8;">Date:</td>
					<td style="color: #cbd5e1;">%s</td>
				</tr>
			</table>
		</div>

		<p style="margin: 0; font-size: 14px; color: #94a3b8;">
			You can download invoices or update payment methods at any time in your workspace billing portal.
		</p>
	`, safeRecipientName, safePlanTier, safeInvoiceNumber, safeAmount, safePaymentID, inv.BillingDate.Format("January 02, 2006"))

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("Invoice %s", safeInvoiceNumber), content, "View Billing Portal", inv.BillingPortalURL, "", DrixyMascotReadingBook, "Drixy Invoice")
	return subject, htmlBody
}
