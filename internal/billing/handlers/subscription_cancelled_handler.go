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

// SubscriptionCancelledHandler processes voluntary cancellations and terminal non-renewals.
type SubscriptionCancelledHandler struct {
	repo        billing.IBillingRepository
	seatManager *SeatManagementHandler
}

// NewSubscriptionCancelledHandler constructs a cancellation handler.
func NewSubscriptionCancelledHandler(
	repo billing.IBillingRepository,
	seatManager *SeatManagementHandler,
) *SubscriptionCancelledHandler {
	return &SubscriptionCancelledHandler{
		repo:        repo,
		seatManager: seatManager,
	}
}

// HandleBillingEvent transitions the organization to community tier upon cancellation.
func (h *SubscriptionCancelledHandler) HandleBillingEvent(ctx context.Context, event billing.BillingWebhookEvent) error {
	var payload billing.SubscriptionCancelledPayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal SubscriptionCancelledPayload: %w", err)
	}

	orgID := event.OrganizationID
	if orgID == [16]byte{} {
		orgID = payload.OrganizationID
	}
	if orgID == [16]byte{} {
		return fmt.Errorf("missing organization_id in subscription cancelled event")
	}

	slog.Warn("Subscription cancelled hook received",
		"org_id", orgID,
		"effective_date", payload.EffectiveDate,
		"reason", payload.Reason,
	)

	// If cancellation is immediate or grace period has elapsed
	now := time.Now().UTC()
	if payload.ImmediateGracePeriod || (!payload.GracePeriodEnd.IsZero() && now.After(payload.GracePeriodEnd)) {
		const communitySeats = 3
		if err := h.repo.UpdateOrganizationPlan(ctx, orgID, "COMMUNITY", communitySeats, []string{}); err != nil {
			return fmt.Errorf("failed to downgrade cancelled subscription to community: %w", err)
		}

		if h.seatManager != nil {
			_, err := h.seatManager.AutoRevokeRemovedLicenseSeats(ctx, orgID, communitySeats)
			if err != nil {
				slog.Error("Auto seat revocation after cancellation encountered error", "org_id", orgID, "error", err)
			}
		}

		slog.Info("Downgraded cancelled tenant to Community tier and reclaimed excess seats", "org_id", orgID)
	}

	return nil
}
