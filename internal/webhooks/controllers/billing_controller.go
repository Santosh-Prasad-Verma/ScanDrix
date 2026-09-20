package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

// BillingController handles outbound billing webhooks from payment processors and billing services.
type BillingController struct {
	repo          *database.Repository
	verifier      *ingestion.WebhookVerifier
	webhookSecret string
}

// NewBillingController creates a new billing webhook controller.
func NewBillingController(
	repo *database.Repository,
	webhookSecret string,
) *BillingController {
	return &BillingController{
		repo:          repo,
		verifier:      ingestion.NewWebhookVerifier(),
		webhookSecret: webhookSecret,
	}
}

// RegisterRoutes registers billing webhook endpoints on a Chi router.
func (c *BillingController) RegisterRoutes(r chi.Router) {
	r.Post("/webhook", c.HandleGenericWebhook)
	r.Post("/payment-failed", c.HandlePaymentFailed)
	r.Post("/trial-expiring", c.HandleTrialExpiring)
	r.Post("/plan-changed", c.HandlePlanChanged)
}

// PaymentFailedBody defines the payload for payment failures.
type PaymentFailedBody struct {
	OrganizationID   string  `json:"organizationId"`
	WorkspaceID      string  `json:"workspaceId"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	FailureReason    string  `json:"failureReason"`
	NextRetryAt      string  `json:"nextRetryAt"`
	UpdatePaymentURL string  `json:"updatePaymentUrl"`
}

// TrialExpiringBody defines the payload for expiring trials.
type TrialExpiringBody struct {
	OrganizationID string `json:"organizationId"`
	WorkspaceID    string `json:"workspaceId"`
	TrialEndsAt    string `json:"trialEndsAt"`
	DaysRemaining  int    `json:"daysRemaining"`
	UpgradeURL     string `json:"upgradeUrl"`
}

// PlanChangedBody defines the payload for plan changes.
type PlanChangedBody struct {
	OrganizationID     string `json:"organizationId"`
	WorkspaceID        string `json:"workspaceId"`
	TeamID             string `json:"teamId"`
	PlanType           string `json:"planType"`
	SubscriptionStatus string `json:"subscriptionStatus"`
}

func (c *BillingController) verifyRequest(r *http.Request, rawBody []byte) (bool, int, string) {
	if c.webhookSecret == "" {
		slog.Error("Billing webhook secret is not configured — refusing billing webhook")
		return false, http.StatusInternalServerError, "Webhook secret not configured"
	}

	sig := r.Header.Get("x-scandrix-signature")
	if sig == "" {
		sig = r.Header.Get("x-razorpay-signature")
	}
	if sig == "" {
		sig = r.Header.Get("x-signature")
	}
	if sig == "" {
		sig = r.Header.Get("X-Signature")
	}
	if sig == "" {
		return false, http.StatusUnauthorized, "Missing signature"
	}

	if !c.verifier.VerifyBilling(sig, rawBody, c.webhookSecret) {
		return false, http.StatusUnauthorized, "Invalid signature"
	}

	return true, http.StatusOK, ""
}

// HandleGenericWebhook routes standard inbound payment webhooks.
func (c *BillingController) HandleGenericWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	if ok, code, reason := c.verifyRequest(r, rawBody); !ok {
		http.Error(w, reason, code)
		return
	}

	// Record billing event to outbox for worker processing
	c.recordBillingOutbox(r.Context(), "billing.webhook_received", rawBody, uuid.Nil)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// HandlePaymentFailed processes billing payment failure notifications.
func (c *BillingController) HandlePaymentFailed(w http.ResponseWriter, r *http.Request) {
	rawBody, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	if ok, code, reason := c.verifyRequest(r, rawBody); !ok {
		http.Error(w, reason, code)
		return
	}

	var body PaymentFailedBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	targetID := body.OrganizationID
	if targetID == "" {
		targetID = body.WorkspaceID
	}
	if targetID == "" {
		http.Error(w, "Missing organizationId", http.StatusBadRequest)
		return
	}

	wsID, _ := uuid.Parse(targetID)
	c.recordBillingOutbox(r.Context(), "billing.payment_failed", rawBody, wsID)

	slog.Info("Recorded billing payment-failed notification", "target_id", targetID, "amount", body.Amount)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// HandleTrialExpiring processes subscription trial expiration notifications.
func (c *BillingController) HandleTrialExpiring(w http.ResponseWriter, r *http.Request) {
	rawBody, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	if ok, code, reason := c.verifyRequest(r, rawBody); !ok {
		http.Error(w, reason, code)
		return
	}

	var body TrialExpiringBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	targetID := body.OrganizationID
	if targetID == "" {
		targetID = body.WorkspaceID
	}
	if targetID == "" {
		http.Error(w, "Missing organizationId", http.StatusBadRequest)
		return
	}

	wsID, _ := uuid.Parse(targetID)
	c.recordBillingOutbox(r.Context(), "billing.trial_expiring", rawBody, wsID)

	slog.Info("Recorded billing trial-expiring notification", "target_id", targetID, "days_remaining", body.DaysRemaining)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// HandlePlanChanged processes plan upgrades/downgrades and triggers limit recalculation.
func (c *BillingController) HandlePlanChanged(w http.ResponseWriter, r *http.Request) {
	rawBody, err := ReadPayload(w, r)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}

	if ok, code, reason := c.verifyRequest(r, rawBody); !ok {
		http.Error(w, reason, code)
		return
	}

	var body PlanChangedBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	targetID := body.OrganizationID
	if targetID == "" {
		targetID = body.WorkspaceID
	}
	if targetID == "" {
		http.Error(w, "Missing organizationId", http.StatusBadRequest)
		return
	}

	wsID, _ := uuid.Parse(targetID)
	c.recordBillingOutbox(r.Context(), "billing.plan_changed", rawBody, wsID)

	slog.Info("Recorded billing plan-changed notification", "target_id", targetID, "plan_type", body.PlanType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (c *BillingController) recordBillingOutbox(ctx context.Context, eventType string, payload []byte, wsID uuid.UUID) {
	if c.repo == nil {
		return
	}
	outboxRec := &models.OutboxRecord{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		EventType:   eventType,
		Payload:     payload,
		Status:      models.OutboxPending,
		CreatedAt:   time.Now().UTC(),
	}
	_ = c.repo.IngestWebhookEventTx(ctx, nil, outboxRec)
}
