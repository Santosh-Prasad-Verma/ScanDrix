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
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// WebhookConfig contains provider webhook signing secrets.
type WebhookConfig struct {
	StripeSigningSecret   string        `json:"stripe_signing_secret"`
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

// WebhookController receives and verifies external billing webhooks from Stripe and Razorpay.
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
	r.Post("/stripe", c.HandleStripeWebhook)
	r.Post("/razorpay", c.HandleRazorpayWebhook)
	r.Post("/scandrix", c.HandleScanDrixWebhook)
}

// HandleStripeWebhook handles Stripe event payloads with v1 HMAC-SHA256 signature verification.
func (c *WebhookController) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	if sigHeader == "" {
		http.Error(w, "Missing Stripe-Signature header", http.StatusUnauthorized)
		return
	}

	if err := c.verifyStripeSignature(rawBody, sigHeader); err != nil {
		slog.Warn("Stripe signature verification failed", "error", err)
		http.Error(w, "Unauthorized signature", http.StatusUnauthorized)
		return
	}

	// Parse Stripe payload
	var stripeEv struct {
		ID   string          `json:"id"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &stripeEv); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	event := c.normalizeStripeEvent(stripeEv.ID, stripeEv.Type, rawBody)

	// Respond 200 OK immediately to satisfy Stripe SLA
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "received", "id": event.ID})

	// Dispatch asynchronously
	if c.dispatcher != nil {
		_ = c.dispatcher.Dispatch(r.Context(), event)
	}
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

func (c *WebhookController) verifyStripeSignature(rawBody []byte, header string) error {
	if c.config.StripeSigningSecret == "" {
		return errors.New("stripe signing secret is not configured")
	}

	var timestampStr string
	var signatures []string

	pairs := strings.Split(header, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		if key == "t" {
			timestampStr = val
		} else if key == "v1" {
			signatures = append(signatures, val)
		}
	}

	if timestampStr == "" || len(signatures) == 0 {
		return errors.New("incomplete Stripe-Signature header")
	}

	tsInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	eventTime := time.Unix(tsInt, 0)
	if time.Since(eventTime).Abs() > c.config.TimestampTolerance {
		return errors.New("stripe signature timestamp outside tolerance window")
	}

	signedPayload := []byte(fmt.Sprintf("%s.%s", timestampStr, string(rawBody)))
	mac := hmac.New(sha256.New, []byte(c.config.StripeSigningSecret))
	mac.Write(signedPayload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expectedMAC)) {
			return nil
		}
	}

	return errors.New("no matching valid stripe signature")
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

func (c *WebhookController) normalizeStripeEvent(id, stripeType string, raw []byte) BillingWebhookEvent {
	var evType BillingEventType
	switch stripeType {
	case "customer.subscription.updated":
		evType = EventPlanChanged
	case "customer.subscription.deleted":
		evType = EventSubscriptionCancelled
	case "invoice.payment_failed":
		evType = EventPaymentFailed
	case "invoice.payment_succeeded":
		evType = EventInvoiceSucceeded
	case "customer.subscription.trial_will_end":
		evType = EventTrialExpiring
	default:
		evType = BillingEventType("billing." + stripeType)
	}

	// Try extracting organization_id from metadata if present
	var parsed struct {
		Data struct {
			Object struct {
				Metadata struct {
					OrganizationID string `json:"organization_id"`
					WorkspaceID    string `json:"workspace_id"`
				} `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &parsed)

	orgID, _ := uuid.Parse(parsed.Data.Object.Metadata.OrganizationID)
	if orgID == uuid.Nil {
		orgID, _ = uuid.Parse(parsed.Data.Object.Metadata.WorkspaceID)
	}

	return BillingWebhookEvent{
		ID:             id,
		Provider:       ProviderStripe,
		EventType:      evType,
		OrganizationID: orgID,
		Timestamp:      time.Now().UTC(),
		RawPayload:     raw,
	}
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
