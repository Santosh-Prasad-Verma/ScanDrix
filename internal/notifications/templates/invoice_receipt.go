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
	safeRecipientEmail := html.EscapeString(inv.RecipientEmail)
	safeOrgName := html.EscapeString(inv.OrganizationName)
	safePaymentID := html.EscapeString(inv.PaymentID)
	safePlanTier := html.EscapeString(inv.PlanTier)
	safePaymentProvider := html.EscapeString(inv.PaymentProvider)
	safeOrderID := html.EscapeString(inv.OrderID)

	subject = fmt.Sprintf("Your ScanDrix Invoice %s (%s)", safeInvoiceNumber, safeAmount)
	preview := fmt.Sprintf("Thank you for your payment. ScanDrix invoice %s for %s has been processed.", safeInvoiceNumber, safeAmount)

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>We have successfully processed your payment. Here is the itemized tax invoice and receipt for your records.</p>

		<!-- Receipt Header Metadata -->
		<table border="0" cellpadding="4" cellspacing="0" width="100%%" style="margin: 20px 0; font-size: 13px; color: #a1a1aa;">
			<tr>
				<td style="width: 50%%;">
					<strong style="color: #ffffff;">Invoice Number:</strong> %s<br>
					<strong style="color: #ffffff;">Date of Issue:</strong> %s<br>
					<strong style="color: #ffffff;">Payment Status:</strong> <span style="display: inline-block; background-color: rgba(201, 243, 107, 0.15); color: #c9f36b; font-weight: 700; padding: 2px 8px; border-radius: 4px; border: 1px solid rgba(201, 243, 107, 0.3); font-size: 11px;">PAID</span>
				</td>
				<td style="width: 50%%; text-align: right;">
					<strong style="color: #ffffff;">Billed To:</strong> %s<br>
					<strong style="color: #ffffff;">Workspace:</strong> %s<br>
					<strong style="color: #ffffff;">Payment ID:</strong> %s
				</td>
			</tr>
		</table>

		<!-- Itemized Charges Table -->
		<table border="0" cellpadding="10" cellspacing="0" width="100%%" style="margin: 20px 0; border-collapse: collapse; border: 1px solid #26282f; font-size: 14px; background-color: #121316;">
			<thead>
				<tr style="background-color: #16171b; border-bottom: 1px solid #26282f; color: #ffffff; font-weight: 600;">
					<th align="left" style="padding: 10px 12px;">Description</th>
					<th align="center" style="padding: 10px 12px; width: 60px;">Qty</th>
					<th align="right" style="padding: 10px 12px; width: 120px;">Amount</th>
				</tr>
			</thead>
			<tbody>
				<tr style="border-bottom: 1px solid #1f2026;">
					<td style="padding: 12px; color: #d4d4d8;">
						<strong style="color: #ffffff;">ScanDrix %s Subscription</strong><br>
						<span style="font-size: 12px; color: #a1a1aa;">10,000,000 monthly tokens, automated AI code reviews, multi-provider routing</span>
					</td>
					<td align="center" style="padding: 12px; color: #d4d4d8;">1</td>
					<td align="right" style="padding: 12px; font-weight: 700; color: #ffffff;">%s</td>
				</tr>
				<tr style="background-color: #16171b; font-weight: 700; color: #ffffff;">
					<td colspan="2" align="right" style="padding: 12px;">Total Paid (%s):</td>
					<td align="right" style="padding: 12px; color: #c9f36b; font-size: 16px;">%s</td>
				</tr>
			</tbody>
		</table>

		<div style="background-color: #16171b; border: 1px solid #26282f; border-radius: 6px; padding: 14px 16px; margin: 20px 0; font-size: 12px; color: #a1a1aa;">
			<strong style="color: #ffffff;">Payment Information:</strong><br>
			Processed securely via %s (Order: %s). Next renewal scheduled for %s.
		</div>

		<p style="font-size: 13px; color: #71717a;">
			You can manage your subscription, add seats, or download VAT/GST breakdown PDFs at any time through the ScanDrix Billing Portal.
		</p>
	`,
		safeRecipientName,
		safeInvoiceNumber,
		inv.BillingDate.Format("Jan 02, 2006"),
		safeRecipientEmail,
		safeOrgName,
		safePaymentID,
		safePlanTier,
		safeAmount,
		safePaymentProvider,
		safeAmount,
		safePaymentProvider,
		safeOrderID,
		inv.NextBillingDate.Format("Jan 02, 2006"),
	)

	htmlBody = RenderBrandLayout(preview, "Payment Receipt & Tax Invoice", content, "View Billing Portal", inv.BillingPortalURL)
	return subject, htmlBody
}
