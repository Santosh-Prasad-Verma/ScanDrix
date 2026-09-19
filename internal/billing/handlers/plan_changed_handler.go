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
	"github.com/scandrix/backend/internal/enterprise/license"
)

// PlanChangedHandler processes subscription plan upgrade and downgrade events.
type PlanChangedHandler struct {
	repo        billing.IBillingRepository
	seatManager *SeatManagementHandler
}

// NewPlanChangedHandler constructs a plan transition handler.
func NewPlanChangedHandler(repo billing.IBillingRepository, seatManager *SeatManagementHandler) *PlanChangedHandler {
	return &PlanChangedHandler{
		repo:        repo,
		seatManager: seatManager,
	}
}

// HandleBillingEvent executes plan modification logic.
func (h *PlanChangedHandler) HandleBillingEvent(ctx context.Context, event billing.BillingWebhookEvent) error {
	var payload billing.PlanChangePayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal PlanChangePayload: %w", err)
	}

	orgID := event.OrganizationID
	if orgID == [16]byte{} {
		orgID = payload.OrganizationID
	}
	if orgID == [16]byte{} {
		return fmt.Errorf("missing organization_id in plan changed event")
	}

	slog.Info("Processing plan changed hook",
		"org_id", orgID,
		"new_tier", payload.NewTier,
		"seat_limit", payload.NewSeatLimit,
	)

	// Determine feature list for new tier
	features := getFeaturesForTier(payload.NewTier)

	// Update plan and seat limits in DB
	if err := h.repo.UpdateOrganizationPlan(ctx, orgID, payload.NewTier, payload.NewSeatLimit, features); err != nil {
		return fmt.Errorf("failed to update organization plan in db: %w", err)
	}

	// If seat count decreased, check and execute auto seat revocation
	if payload.AutoPruneSeats || payload.NewSeatLimit > 0 {
		if h.seatManager != nil {
			_, err := h.seatManager.AutoRevokeRemovedLicenseSeats(ctx, orgID, payload.NewSeatLimit)
			if err != nil {
				slog.Error("Auto seat revocation encountered an error", "org_id", orgID, "error", err)
			}
		}
	}

	return nil
}

func getFeaturesForTier(tier string) []string {
	switch license.LicenseTier(tier) {
	case license.TierEnterprise:
		return []string{
			string(license.FeatureSSOSAML),
			string(license.FeatureSCIM),
			string(license.FeatureAuditWarehouse),
			string(license.FeatureMultiAgentDeliberation),
			string(license.FeatureBYOK),
			string(license.FeatureCustomRules),
			string(license.FeatureDORAMetrics),
		}
	case license.TierTeam:
		return []string{
			string(license.FeatureMultiAgentDeliberation),
			string(license.FeatureCustomRules),
			string(license.FeatureDORAMetrics),
		}
	case license.TierDeveloper:
		return []string{
			string(license.FeatureCustomRules),
		}
	default:
		return []string{}
	}
}
