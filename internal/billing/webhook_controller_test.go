// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func computeHMACSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookController_RazorpaySignature(t *testing.T) {
	rzpSecret := "rzp_sec_test_razorpay_secret_999"
	cfg := WebhookConfig{
		RazorpayWebhookSecret: rzpSecret,
	}

	idemp := NewMemoryIdempotencyStore()
	dispatcher := NewHooksDispatcher(idemp, nil, DefaultHooksDispatcherConfig())
	defer dispatcher.Stop()

	controller := NewWebhookController(dispatcher, cfg)
	r := chi.NewRouter()
	controller.RegisterRoutes(r)

	payload := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"notes":{"organization_id":"00000000-0000-0000-0000-000000000002"}}}}}`)

	// 1. Valid signature
	validSig := computeHMACSignature(payload, rzpSecret)
	req, _ := http.NewRequest(http.MethodPost, "/razorpay", bytes.NewReader(payload))
	req.Header.Set("X-Razorpay-Signature", validSig)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"received"`)

	// 2. Invalid signature -> 401
	req2, _ := http.NewRequest(http.MethodPost, "/razorpay", bytes.NewReader(payload))
	req2.Header.Set("X-Razorpay-Signature", "invalid_sig_abc123")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusUnauthorized, rec2.Code)
}

func TestWebhookController_ScanDrixSignature(t *testing.T) {
	scandrixSecret := "scandrix_wh_sec_dev_2026"
	cfg := WebhookConfig{
		ScanDrixWebhookSecret: scandrixSecret,
	}

	idemp := NewMemoryIdempotencyStore()
	dispatcher := NewHooksDispatcher(idemp, nil, DefaultHooksDispatcherConfig())
	defer dispatcher.Stop()

	controller := NewWebhookController(dispatcher, cfg)
	r := chi.NewRouter()
	controller.RegisterRoutes(r)

	payload := []byte(`{"id":"sd_evt_1","provider":"manual","event_type":"billing.plan_changed","organization_id":"00000000-0000-0000-0000-000000000003"}`)

	validSig := computeHMACSignature(payload, scandrixSecret)
	req, _ := http.NewRequest(http.MethodPost, "/scandrix", bytes.NewReader(payload))
	req.Header.Set("X-ScanDrix-Signature", validSig)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"received"`)
}
