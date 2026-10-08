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
	preview := fmt.Sprintf("We were unable to process payment for your ScanDrix %s subscription.", safeTier)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 16px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">We were unable to process the renewal payment for your <strong>%s</strong> subscription on workspace <strong>%s</strong>.</p>

		<div style="background-color: #fef2f2; border: 1px solid #fee2e2; border-left: 3px solid #ef4444; border-radius: 4px; padding: 14px 18px; margin: 20px 0; font-size: 14px; color: #991b1b;">
			<strong>Reason:</strong> %s (Order: %s)
		</div>

		<p style="margin: 0; font-size: 14px; color: #6b7280;">
			Please update your payment method to ensure automated pull request reviews continue uninterrupted.
		</p>
	`, safeSubscriber, safeTier, safeOrg, safeReason, safeOrderID)

	htmlBody = RenderBrandLayout(preview, "Payment Failed", content, "Update Payment Method", retryURL)
	return subject, htmlBody
}
