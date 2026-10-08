package templates

import (
	"fmt"
	"html"
)

// RenderPaymentFailed generates the notification when a subscription payment or renewal fails.
func RenderPaymentFailed(subscriberName, orgName, planTier, orderID, failureReason, retryURL string) (subject, htmlBody string) {
	safeSubscriber := html.EscapeString(subscriberName)
	safeOrg := html.EscapeString(orgName)
	safeTier := html.EscapeString(planTier)
	safeOrderID := html.EscapeString(orderID)
	safeReason := html.EscapeString(failureReason)

	subject = fmt.Sprintf("Action Required: Payment failed for ScanDrix %s", safeOrg)
	preview := fmt.Sprintf("We were unable to process payment for your ScanDrix %s subscription. Please update your payment details.", safeTier)

	content := fmt.Sprintf(`
		<p>Hi <strong>%s</strong>,</p>
		<p>We recently attempted to process payment for your <strong>%s</strong> subscription for workspace <strong>%s</strong>, but the transaction was declined.</p>

		<div style="background-color: #1a1415; border: 1px solid #3b1d22; border-left: 4px solid #ef4444; border-radius: 0 8px 8px 0; padding: 18px 20px; margin: 24px 0;">
			<h4 style="margin: 0 0 10px 0; color: #f87171; font-size: 14px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em;">Transaction Details:</h4>
			<ul style="margin: 0; padding-left: 20px; color: #fca5a5; font-size: 13px; line-height: 1.6;">
				<li><strong>Order ID:</strong> %s</li>
				<li><strong>Reason:</strong> %s</li>
			</ul>
		</div>

		<p style="color: #a1a1aa; font-size: 14px;">
			To prevent disruption to your team's pull request reviews and to maintain access to frontier AI models, please update your payment method or retry the checkout below.
		</p>
	`, safeSubscriber, safeTier, safeOrg, safeOrderID, safeReason)

	htmlBody = RenderBrandLayout(preview, "Payment Failed — Update Billing Info", content, "Retry Payment Now", retryURL)
	return subject, htmlBody
}
