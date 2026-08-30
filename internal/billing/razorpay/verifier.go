package razorpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// VerifyPaymentSignature cryptographically validates the HMAC-SHA256 signature
// returned by Razorpay Checkout upon successful payment capture.
//
// Expected signature = HMAC-SHA256(order_id + "|" + payment_id, key_secret)
// Evaluation is performed in constant time to prevent timing-attack vulnerability.
func VerifyPaymentSignature(orderID, paymentID, signature, keySecret string) bool {
	if orderID == "" || paymentID == "" || signature == "" || keySecret == "" {
		return false
	}

	payload := orderID + "|" + paymentID
	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// VerifyWebhookSignature cryptographically validates the X-Razorpay-Signature
// header on incoming webhook deliveries.
//
// Expected signature = HMAC-SHA256(raw_request_body, webhook_secret)
func VerifyWebhookSignature(body []byte, signature, webhookSecret string) bool {
	if len(body) == 0 || signature == "" || webhookSecret == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}
