// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/billing"
)

// PaymentFailedHandler executes actions when an invoice or credit card transaction fails.
type PaymentFailedHandler struct {
	repo    billing.IBillingRepository
	dunning *billing.DunningManager
}

// NewPaymentFailedHandler creates a payment failure handler.
func NewPaymentFailedHandler(repo billing.IBillingRepository, dunning *billing.DunningManager) *PaymentFailedHandler {
	return &PaymentFailedHandler{
		repo:    repo,
		dunning: dunning,
	}
}

// HandleBillingEvent coordinates dunning escalation and notification logging.
func (h *PaymentFailedHandler) HandleBillingEvent(ctx context.Context, event billing.BillingWebhookEvent) error {
	var payload billing.PaymentFailurePayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal PaymentFailurePayload: %w", err)
	}

	orgID := event.OrganizationID
	if orgID == [16]byte{} {
		orgID = payload.OrganizationID
	}
	if orgID == [16]byte{} {
		return fmt.Errorf("missing organization_id in payment failure event")
	}

	slog.Warn("Payment failure hook received",
		"org_id", orgID,
		"invoice_id", payload.InvoiceID,
		"amount", payload.Amount,
		"reason", payload.FailureReason,
		"attempt", payload.AttemptCount,
	)

	// Record failure record in DB
	failureRec := billing.PaymentFailureRecord{
		OrganizationID: orgID,
		Provider:       event.Provider,
		InvoiceID:      payload.InvoiceID,
		Amount:         payload.Amount,
		Currency:       payload.Currency,
		FailureReason:  payload.FailureReason,
		AttemptCount:   payload.AttemptCount,
		NextRetryAt:    payload.NextRetryAt,
		FailedAt:       time.Now().UTC(),
	}
	if err := h.repo.RecordFailedPayment(ctx, failureRec); err != nil {
		slog.Error("Failed to persist payment failure record", "org_id", orgID, "error", err)
	}

	// Escalate Dunning State
	if h.dunning != nil {
		transition, err := h.dunning.HandlePaymentFailure(ctx, payload)
		if err != nil {
			return fmt.Errorf("dunning manager failed: %w", err)
		}

		slog.Info("Dunning transition evaluated",
			"org_id", orgID,
			"new_state", transition.NewState,
			"reason", transition.Reason,
		)
	}

	return nil
}
