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

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/billing"
)

// InvoiceSucceededHandler handles settled invoices, token quota refreshes, and dunning resolutions.
type InvoiceSucceededHandler struct {
	repo    billing.IBillingRepository
	dunning *billing.DunningManager
}

// NewInvoiceSucceededHandler creates an invoice success handler.
func NewInvoiceSucceededHandler(repo billing.IBillingRepository, dunning *billing.DunningManager) *InvoiceSucceededHandler {
	return &InvoiceSucceededHandler{
		repo:    repo,
		dunning: dunning,
	}
}

// HandleBillingEvent records payment receipt, resets quota counters, and clears dunning status.
func (h *InvoiceSucceededHandler) HandleBillingEvent(ctx context.Context, event billing.BillingWebhookEvent) error {
	var payload billing.InvoiceSucceededPayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal InvoiceSucceededPayload: %w", err)
	}

	orgID := event.OrganizationID
	if orgID == [16]byte{} {
		orgID = payload.OrganizationID
	}
	if orgID == [16]byte{} {
		return fmt.Errorf("missing organization_id in invoice succeeded event")
	}

	slog.Info("Invoice succeeded hook received",
		"org_id", orgID,
		"invoice_id", payload.InvoiceID,
		"amount", payload.Amount,
		"currency", payload.Currency,
	)

	// Record transaction
	tx := billing.BillingTransactionRecord{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Provider:       event.Provider,
		TransactionID:  event.ID,
		InvoiceID:      payload.InvoiceID,
		Amount:         payload.Amount,
		Currency:       payload.Currency,
		Status:         "succeeded",
		Timestamp:      time.Now().UTC(),
	}
	if err := h.repo.RecordBillingTransaction(ctx, tx); err != nil {
		slog.Error("Failed to record billing transaction", "org_id", orgID, "error", err)
	}

	// Reset monthly token counter if new billing cycle started
	if payload.ResetTokens {
		if err := h.repo.ResetMonthlyTokenUsage(ctx, orgID); err != nil {
			slog.Error("Failed to reset monthly token budget", "org_id", orgID, "error", err)
		} else {
			slog.Info("Reset monthly AI review token budget", "org_id", orgID)
		}
	}

	// Resolve dunning status if tenant was overdue
	if h.dunning != nil {
		transition, err := h.dunning.HandlePaymentSuccess(ctx, orgID, payload.InvoiceID)
		if err != nil {
			slog.Error("Failed to clear dunning status upon payment success", "org_id", orgID, "error", err)
		} else if transition != nil {
			slog.Info("Organization dunning delinquency resolved; returned to Good Standing",
				"org_id", orgID,
				"new_state", transition.NewState,
			)
		}
	}

	return nil
}
