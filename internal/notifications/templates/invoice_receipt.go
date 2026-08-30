package templates

import (
	"fmt"
)

// RenderInvoiceReceipt generates the itemized payment receipt and invoice email.
func RenderInvoiceReceipt(inv InvoiceDetails) (subject, htmlBody string) {
	subject = fmt.Sprintf("Your ScanDrix Invoice %s (%s)", inv.InvoiceNumber, inv.AmountFormatted)
	preview := fmt.Sprintf("Thank you for your payment. ScanDrix invoice %s for %s has been processed.", inv.InvoiceNumber, inv.AmountFormatted)

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>We have successfully processed your payment. Here is the itemized tax invoice and receipt for your records.</p>

		<!-- Receipt Header Metadata -->
		<table border="0" cellpadding="4" cellspacing="0" width="100%%" style="margin: 20px 0; font-size: 13px; color: #4b5563;">
			<tr>
				<td style="width: 50%%;">
					<strong>Invoice Number:</strong> %s<br>
					<strong>Date of Issue:</strong> %s<br>
					<strong>Payment Status:</strong> <span style="color: #059669; font-weight: 700;">PAID</span>
				</td>
				<td style="width: 50%%; text-align: right;">
					<strong>Billed To:</strong> %s<br>
					<strong>Workspace:</strong> %s<br>
					<strong>Payment ID:</strong> %s
				</td>
			</tr>
		</table>

		<!-- Itemized Charges Table -->
		<table border="0" cellpadding="10" cellspacing="0" width="100%%" style="margin: 20px 0; border-collapse: collapse; border: 1px solid #e5e7eb; font-size: 14px;">
			<thead>
				<tr style="background-color: #f9fafb; border-bottom: 1px solid #e5e7eb; color: #374151; font-weight: 600;">
					<th align="left" style="padding: 10px 12px;">Description</th>
					<th align="center" style="padding: 10px 12px; width: 60px;">Qty</th>
					<th align="right" style="padding: 10px 12px; width: 120px;">Amount</th>
				</tr>
			</thead>
			<tbody>
				<tr style="border-bottom: 1px solid #f3f4f6;">
					<td style="padding: 12px;">
						<strong>ScanDrix %s Subscription</strong><br>
						<span style="font-size: 12px; color: #6b7280;">10,000,000 monthly tokens, automated AI code reviews, multi-provider routing</span>
					</td>
					<td align="center" style="padding: 12px;">1</td>
					<td align="right" style="padding: 12px; font-weight: 600;">%s</td>
				</tr>
				<tr style="background-color: #f9fafb; font-weight: 700; color: #111827;">
					<td colspan="2" align="right" style="padding: 12px;">Total Paid (%s):</td>
					<td align="right" style="padding: 12px; color: #111827; font-size: 16px;">%s</td>
				</tr>
			</tbody>
		</table>

		<div style="background-color: #f9fafb; border-radius: 6px; padding: 14px 16px; margin: 20px 0; font-size: 12px; color: #6b7280;">
			<strong>Payment Information:</strong><br>
			Processed securely via %s (Order: %s). Next renewal scheduled for %s.
		</div>

		<p style="font-size: 13px; color: #6b7280;">
			You can manage your subscription, add seats, or download VAT/GST breakdown PDFs at any time through the ScanDrix Billing Portal.
		</p>
	`,
		inv.RecipientName,
		inv.InvoiceNumber,
		inv.BillingDate.Format("Jan 02, 2006"),
		inv.RecipientEmail,
		inv.OrganizationName,
		inv.PaymentID,
		inv.PlanTier,
		inv.AmountFormatted,
		inv.PaymentProvider,
		inv.AmountFormatted,
		inv.PaymentProvider,
		inv.OrderID,
		inv.NextBillingDate.Format("Jan 02, 2006"),
	)

	htmlBody = RenderBrandLayout(preview, "Payment Receipt & Tax Invoice", content, "View Billing Portal", inv.BillingPortalURL)
	return subject, htmlBody
}
