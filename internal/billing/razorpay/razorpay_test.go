package razorpay_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/llm"
)

func TestPaymentSignatureVerification(t *testing.T) {
	secret := "test_secret_key_123456"
	orderID := "order_test_987654"
	paymentID := "pay_test_123456"

	// Compute expected HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(orderID + "|" + paymentID))
	validSig := hex.EncodeToString(mac.Sum(nil))

	// 1. Verify valid signature passes
	if !razorpay.VerifyPaymentSignature(orderID, paymentID, validSig, secret) {
		t.Fatalf("expected valid signature to pass verification")
	}

	// 2. Verify tampered signature fails
	tamperedSig := validSig[:len(validSig)-4] + "0000"
	if razorpay.VerifyPaymentSignature(orderID, paymentID, tamperedSig, secret) {
		t.Fatalf("expected tampered signature to fail verification")
	}

	// 3. Verify wrong secret fails
	if razorpay.VerifyPaymentSignature(orderID, paymentID, validSig, "wrong_secret") {
		t.Fatalf("expected verification with wrong secret to fail")
	}
}

func TestWebhookSignatureVerification(t *testing.T) {
	secret := "webhook_secret_abcdef"
	payload := []byte(`{"event":"order.paid","account_id":"acc_123"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	if !razorpay.VerifyWebhookSignature(payload, validSig, secret) {
		t.Fatalf("expected valid webhook signature to pass")
	}

	if razorpay.VerifyWebhookSignature(payload, "invalid_signature", secret) {
		t.Fatalf("expected invalid webhook signature to fail")
	}
}

func TestBillingServiceOrderCreationAndUpgrade(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()
	secret := "sec_key_xyz987"
	limiter := llm.NewTokenBudgetLimiter()

	// Mock Razorpay Orders API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "order_test_rzp_999",
			"entity": "order",
			"amount": 249900,
			"amount_due": 249900,
			"currency": "INR",
			"receipt": "rcpt_test_001",
			"status": "created",
			"attempts": 0,
			"notes": {"plan_tier": "TEAM"},
			"created_at": 1725000000
		}`))
	}))
	defer mockServer.Close()

	client := razorpay.NewRazorpayClient("rzp_test_key", secret)
	client.SetBaseURL(mockServer.URL)

	noopMailer := mailer.NewNoopSender()
	svc := razorpay.NewBillingService(nil, limiter, noopMailer, "https://app.scandrix.dev", "rzp_test_key", secret, "wh_sec_123")
	svc.SetClient(client)

	// 1. Create Order
	orderResp, err := svc.CreateSubscriptionOrder(ctx, wsID, license.TierTeam, "INR")
	if err != nil {
		t.Fatalf("failed creating order: %v", err)
	}
	if orderResp.Amount != 249900 {
		t.Fatalf("expected 249900 paise for Pro INR, got %d", orderResp.Amount)
	}
	if orderResp.Currency != "INR" {
		t.Fatalf("expected INR, got %s", orderResp.Currency)
	}

	// 2. Generate valid signature
	paymentID := "pay_test_captured_001"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(orderResp.OrderID + "|" + paymentID))
	validSig := hex.EncodeToString(mac.Sum(nil))

	// 3. Verify and Upgrade
	lic, err := svc.VerifyAndUpgrade(ctx, wsID, razorpay.VerifyPaymentRequest{
		OrderID:   orderResp.OrderID,
		PaymentID: paymentID,
		Signature: validSig,
		PlanTier:  "TEAM",
	})
	if err != nil {
		t.Fatalf("failed verifying payment and upgrading: %v", err)
	}

	if lic.PlanTier != "TEAM" {
		t.Fatalf("expected upgraded plan TEAM, got %s", lic.PlanTier)
	}
	if lic.TotalSeats != 25 {
		t.Fatalf("expected 25 seats for Team/Pro tier, got %d", lic.TotalSeats)
	}

	// 4. Verify token budget limiter upgraded to 10M tokens
	usedMonth, maxMonth, _, maxMin, exists := limiter.GetUsage(wsID)
	if !exists {
		t.Fatalf("expected workspace budget to exist in limiter")
	}
	if maxMonth != 10_000_000 {
		t.Fatalf("expected 10M monthly limit, got %d", maxMonth)
	}
	if maxMin != 500_000 {
		t.Fatalf("expected 500K burst limit, got %d", maxMin)
	}
	if usedMonth != 0 {
		t.Fatalf("expected 0 used tokens initially")
	}

	// 5. Verify automatic email triggers: invoice & welcome email dispatched
	if noopMailer.LastPlanTier != "TEAM" {
		t.Fatalf("expected welcome email for TEAM plan, got %s", noopMailer.LastPlanTier)
	}
	if noopMailer.LastInvoice == nil {
		t.Fatalf("expected invoice email to be dispatched")
	}
	if noopMailer.LastInvoice.OrderID != orderResp.OrderID {
		t.Fatalf("expected invoice OrderID %s, got %s", orderResp.OrderID, noopMailer.LastInvoice.OrderID)
	}
	if noopMailer.LastInvoice.PaymentID != paymentID {
		t.Fatalf("expected invoice PaymentID %s, got %s", paymentID, noopMailer.LastInvoice.PaymentID)
	}
}
