// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package integration_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/billing/razorpay"
	"github.com/scandrix/backend/internal/llm"
)

// TestConcurrentRazorpayWebhookIdempotency validates §14 Phase 3:
// Fires 10 concurrent identical Razorpay payment webhooks and verifies
// that all 10 succeed idempotently without race conditions or crashes.
func TestConcurrentRazorpayWebhookIdempotency(t *testing.T) {
	wsID := uuid.New()
	orderID := "order_phase3_concurrent_123"
	paymentID := "pay_phase3_concurrent_456"
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

	payload := []byte(fmt.Sprintf(`{
		"event": "order.paid",
		"payload": {
			"payment": {
				"entity": {
					"id": "%s",
					"order_id": "%s",
					"amount": 149900,
					"currency": "INR",
					"status": "captured",
					"notes": {
						"workspace_id": "%s",
						"plan_tier": "TEAM"
					}
				}
			},
			"order": {
				"entity": {
					"id": "%s",
					"amount": 149900,
					"status": "paid",
					"notes": {
						"workspace_id": "%s",
						"plan_tier": "TEAM"
					}
				}
			}
		},
		"created_at": 1725300000
	}`, paymentID, orderID, wsID.String(), orderID, wsID.String()))

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	// Setup BillingController HTTP router to test live concurrent HTTP ingress
	billingController := controllers.NewBillingController(billingSvc, nil, limiter)
	r := chi.NewRouter()
	r.Mount("/api/v1/billing", billingController.Routes())

	concurrentRequests := 10
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errorCount atomic.Int32

	ctx := context.Background()

	// 1. Direct Service Level Concurrent Stress
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := billingSvc.ProcessWebhook(ctx, payload, signature)
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if success := successCount.Load(); success != int32(concurrentRequests) {
		t.Fatalf("expected %d successful concurrent webhook executions, got %d (errors: %d)",
			concurrentRequests, success, errorCount.Load())
	}

	// 2. HTTP Ingress Level Concurrent Stress
	var httpSuccessCount atomic.Int32
	var httpFailCount atomic.Int32

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/razorpay/webhook", bytes.NewReader(payload))
			req.Header.Set("X-Razorpay-Signature", signature)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code == http.StatusOK {
				httpSuccessCount.Add(1)
			} else {
				httpFailCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if httpSuccess := httpSuccessCount.Load(); httpSuccess != int32(concurrentRequests) {
		t.Fatalf("expected %d HTTP 200 responses, got %d (failures: %d)",
			concurrentRequests, httpSuccess, httpFailCount.Load())
	}
}

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
