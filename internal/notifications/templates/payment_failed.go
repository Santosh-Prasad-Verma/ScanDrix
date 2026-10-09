package templates

import (
	"fmt"
	"html"
)

// RenderPaymentFailed generates a clean notification when a subscription payment fails.
func RenderPaymentFailed(subscriberName, orgName, planTier, orderID, failureReason, retryURL string) (subject, htmlBody string) {
	safeSubscriber := html.EscapeString(subscriberName)
	safeOrg := html.EscapeString(orgName)
	safeTier := html.EscapeString(planTier)
	safeReason := html.EscapeString(failureReason)

	subject = fmt.Sprintf("Action Required: Payment failed for ScanDrix %s", safeOrg)
	preview := fmt.Sprintf("We were unable to process payment for your ScanDrix %s subscription.", safeTier)

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">We were unable to process the payment for your <strong>%s</strong> subscription on workspace <strong>%s</strong>.</p>
		<p style="margin: 0 0 16px 0; font-size: 14px; color: #f87171;">Reason: %s</p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">Please update your payment method to ensure continuous automated code reviews.</p>
	`, safeSubscriber, safeTier, safeOrg, safeReason)

	htmlBody = RenderBrandLayoutWithHero(preview, "Payment Failed", content, "Update Payment Method", retryURL, "", DrixyMascotCuriousLooking, "Drixy Payment Notice")
	return subject, htmlBody
}
