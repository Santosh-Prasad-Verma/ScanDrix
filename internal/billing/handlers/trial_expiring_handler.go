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

	"github.com/scandrix/backend/internal/billing"
)

// TrialExpiringHandler alerts owners when an enterprise evaluation trial nears conclusion.
type TrialExpiringHandler struct {
	repo billing.IBillingRepository
}

// NewTrialExpiringHandler creates a trial expiration handler.
func NewTrialExpiringHandler(repo billing.IBillingRepository) *TrialExpiringHandler {
	return &TrialExpiringHandler{
		repo: repo,
	}
}

// HandleBillingEvent processes trial expiration countdowns.
func (h *TrialExpiringHandler) HandleBillingEvent(ctx context.Context, event billing.BillingWebhookEvent) error {
	var payload billing.TrialExpiringPayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal TrialExpiringPayload: %w", err)
	}

	orgID := event.OrganizationID
	if orgID == [16]byte{} {
		orgID = payload.OrganizationID
	}
	if orgID == [16]byte{} {
		return fmt.Errorf("missing organization_id in trial expiring event")
	}

	slog.Info("Trial expiring notification received",
		"org_id", orgID,
		"days_remaining", payload.DaysRemaining,
		"trial_ends_at", payload.TrialEndsAt,
		"upgrade_url", payload.UpgradeURL,
	)

	// If trial reached 0 days, schedule downgrade to community
	if payload.DaysRemaining <= 0 {
		if err := h.repo.UpdateOrganizationPlan(ctx, orgID, "COMMUNITY", 3, []string{}); err != nil {
			return fmt.Errorf("failed to downgrade expired trial to community: %w", err)
		}
		slog.Info("Downgraded expired trial tenant to Community tier", "org_id", orgID)
	}

	return nil
}
