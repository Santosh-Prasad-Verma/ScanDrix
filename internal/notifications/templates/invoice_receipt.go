package templates

import (
	"fmt"
	"html"
)

// RenderInvoiceReceipt generates the itemized payment receipt and invoice email.
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

		<div style="background-color: #f9fafb; border: 1px solid #e5e7eb; border-radius: 6px; padding: 18px 20px; margin: 24px 0;">
			<table border="0" cellpadding="6" cellspacing="0" width="100%%" style="font-size: 14px; color: #374151;">
				<tr>
					<td style="color: #6b7280; width: 40%%;">Invoice:</td>
					<td style="font-weight: 600; color: #111827;">%s</td>
				</tr>
				<tr>
					<td style="color: #6b7280;">Amount Paid:</td>
					<td style="font-weight: 700; color: #111827;">%s <span style="display: inline-block; background-color: #d1fae5; color: #065f46; font-size: 11px; font-weight: 700; padding: 2px 6px; border-radius: 4px; margin-left: 6px;">PAID</span></td>
				</tr>
				<tr>
					<td style="color: #6b7280;">Payment ID:</td>
					<td style="font-family: monospace; font-size: 13px; color: #4b5563;">%s</td>
				</tr>
				<tr>
					<td style="color: #6b7280;">Date:</td>
					<td style="color: #4b5563;">%s</td>
				</tr>
			</table>
		</div>

		<p style="margin: 0; font-size: 14px; color: #6b7280;">
			You can download invoices or update payment methods at any time in your workspace billing portal.
		</p>
	`, safeRecipientName, safePlanTier, safeInvoiceNumber, safeAmount, safePaymentID, inv.BillingDate.Format("January 02, 2006"))

	htmlBody = RenderBrandLayout(preview, fmt.Sprintf("Invoice %s", safeInvoiceNumber), content, "View Billing Portal", inv.BillingPortalURL)
	return subject, htmlBody
}
