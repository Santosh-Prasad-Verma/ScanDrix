package templates

import (
	"fmt"
	"html"
)

// RenderInvoiceReceipt generates a clean invoice receipt email without bloated tables or loud badges.
func RenderInvoiceReceipt(inv InvoiceDetails) (subject, htmlBody string) {
	safeInvoiceNumber := html.EscapeString(inv.InvoiceNumber)
	safeAmount := html.EscapeString(inv.AmountFormatted)
	safeRecipientName := html.EscapeString(inv.RecipientName)
	safePaymentID := html.EscapeString(inv.PaymentID)
	safePlanTier := html.EscapeString(inv.PlanTier)

	subject = fmt.Sprintf("Your ScanDrix Invoice %s (%s)", safeInvoiceNumber, safeAmount)
	preview := fmt.Sprintf("Thank you for your payment. ScanDrix invoice %s for %s has been processed.", safeInvoiceNumber, safeAmount)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">Thank you for your payment. Your invoice for <strong>%s</strong> has been processed successfully (Status: <strong>PAID</strong>).</p>
		<p style="margin: 0 0 16px 0; font-size: 14px; color: #ffffff; line-height: 1.6;">
			<strong>Invoice:</strong> %s<br>
			<strong>Amount:</strong> %s<br>
			<strong>Payment ID:</strong> <code>%s</code><br>
			<strong>Date:</strong> %s
		</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">You can download invoice PDFs and manage billing details anytime in your workspace portal.</p>
	`, safeRecipientName, safePlanTier, safeInvoiceNumber, safeAmount, safePaymentID, inv.BillingDate.Format("Jan 02, 2006"))

	htmlBody = RenderBrandLayoutWithHero(preview, fmt.Sprintf("Invoice %s", safeInvoiceNumber), content, "View Billing Portal", inv.BillingPortalURL, "", DrixyMascotReadingBook, "Drixy Invoice")
	return subject, htmlBody
}
