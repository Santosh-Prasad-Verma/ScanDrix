// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package integration_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/llm"
)

// Concurrent webhook ingress is covered by
// TestBillingWebhookIngressIsIdempotentUnderConcurrency in
// internal/billing/razorpay, because the endpoint is no longer satisfiable
// without durable storage: handleWebhook returns 503 when the outbox is
// unavailable, and ProcessVerifiedWebhook refuses to grant a plan without a
// stored billing transaction to resolve the tenant from. Both require a real
// PostgreSQL schema, so the concurrency assertions moved to the suite that
// builds one.

// TestTamperedRazorpayWebhookRejected validates fail-closed security for invalid webhook signatures.
func TestTamperedRazorpayWebhookRejected(t *testing.T) {
	webhookSecret := "phase3_razorpay_webhook_secret_key_888"
	limiter := llm.NewTokenBudgetLimiter()
	billingSvc := razorpay.NewBillingService(
		nil,
		limiter,
		nil,
		"http://localhost:3000",
		"rzp_test_key",
		"rzp_test_secret",
		webhookSecret,
	)

	payload := []byte(`{"event":"order.paid"}`)
	invalidSig := "tampered_signature_hex_123456"

	err := billingSvc.ProcessWebhook(context.Background(), payload, invalidSig)
	if err == nil {
		t.Fatal("SECURITY VIOLATION: ProcessWebhook accepted tampered signature!")
	}
}
