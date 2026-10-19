// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// WebhookConfig contains provider webhook signing secrets.
type WebhookConfig struct {
	RazorpayWebhookSecret string        `json:"razorpay_webhook_secret"`
	ScanDrixWebhookSecret string        `json:"scandrix_webhook_secret"`
	TimestampTolerance    time.Duration `json:"timestamp_tolerance"`
}

// DefaultWebhookConfig provides sensible defaults.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		TimestampTolerance: 5 * time.Minute,
	}
}

// WebhookController receives and verifies external billing webhooks from Razorpay and ScanDrix.
type WebhookController struct {
	dispatcher *HooksDispatcher
	config     WebhookConfig
}

// NewWebhookController creates a new webhook controller.
func NewWebhookController(dispatcher *HooksDispatcher, config WebhookConfig) *WebhookController {
	if config.TimestampTolerance <= 0 {
		config.TimestampTolerance = 5 * time.Minute
	}
	return &WebhookController{
		dispatcher: dispatcher,
		config:     config,
	}
}

// RegisterRoutes binds HTTP routes onto a Chi router.
func (c *WebhookController) RegisterRoutes(r chi.Router) {
	r.Post("/razorpay", c.HandleRazorpayWebhook)
	r.Post("/scandrix", c.HandleScanDrixWebhook)
}

// HandleRazorpayWebhook handles Razorpay event payloads with HMAC-SHA256 verification.
func (c *WebhookController) HandleRazorpayWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	sigHeader := r.Header.Get("X-Razorpay-Signature")
	if sigHeader == "" {
		http.Error(w, "Missing X-Razorpay-Signature header", http.StatusUnauthorized)
		return
	}

	if err := c.verifyRazorpaySignature(rawBody, sigHeader); err != nil {
		slog.Warn("Razorpay signature verification failed", "error", err)
		http.Error(w, "Unauthorized signature", http.StatusUnauthorized)
		return
	}

	var rzpEv struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(rawBody, &rzpEv); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	event := c.normalizeRazorpayEvent(rzpEv.Event, rawBody)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "received", "id": event.ID})

	if c.dispatcher != nil {
		_ = c.dispatcher.Dispatch(r.Context(), event)
	}
}

// HandleScanDrixWebhook handles direct ScanDrix platform billing payloads.
func (c *WebhookController) HandleScanDrixWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-ScanDrix-Signature")
	if sig == "" {
		sig = r.Header.Get("x-scandrix-signature")
	}
	if sig == "" {
		http.Error(w, "Missing signature header", http.StatusUnauthorized)
		return
	}

	if err := c.verifyGenericSignature(rawBody, sig, c.config.ScanDrixWebhookSecret); err != nil {
		http.Error(w, "Unauthorized signature", http.StatusUnauthorized)
		return
	}

	var event BillingWebhookEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		http.Error(w, "Invalid billing webhook JSON", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "received", "id": event.ID})

	if c.dispatcher != nil {
		_ = c.dispatcher.Dispatch(r.Context(), event)
	}
}

func (c *WebhookController) verifyRazorpaySignature(rawBody []byte, signature string) error {
	if c.config.RazorpayWebhookSecret == "" {
		return errors.New("razorpay webhook secret is not configured")
	}
	return c.verifyGenericSignature(rawBody, signature, c.config.RazorpayWebhookSecret)
}

func (c *WebhookController) verifyGenericSignature(rawBody []byte, signature string, secret string) error {
	if secret == "" {
		return errors.New("signing secret is empty")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedMAC)) {
		return errors.New("invalid hmac signature")
	}
	return nil
}

func (c *WebhookController) normalizeRazorpayEvent(rzpEvent string, raw []byte) BillingWebhookEvent {
	var evType BillingEventType
	switch rzpEvent {
	case "subscription.charged", "payment.captured":
		evType = EventInvoiceSucceeded
	case "subscription.cancelled":
		evType = EventSubscriptionCancelled
	case "payment.failed":
		evType = EventPaymentFailed
	case "subscription.updated":
		evType = EventPlanChanged
	default:
		evType = BillingEventType("billing." + rzpEvent)
	}

	var parsed struct {
		Payload struct {
			Payment struct {
				Entity struct {
					Notes struct {
						OrganizationID string `json:"organization_id"`
					} `json:"notes"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	_ = json.Unmarshal(raw, &parsed)
	orgID, _ := uuid.Parse(parsed.Payload.Payment.Entity.Notes.OrganizationID)

	return BillingWebhookEvent{
		ID:             fmt.Sprintf("rzp_%d", time.Now().UnixNano()),
		Provider:       ProviderRazorpay,
		EventType:      evType,
		OrganizationID: orgID,
		Timestamp:      time.Now().UTC(),
		RawPayload:     raw,
	}
}
