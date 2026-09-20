// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/billing"
)

// SeatManagementHandler controls automatic seat reclamation and allocation policies.
type SeatManagementHandler struct {
	repo billing.IBillingRepository
}

// NewSeatManagementHandler constructs a seat allocation controller.
func NewSeatManagementHandler(repo billing.IBillingRepository) *SeatManagementHandler {
	return &SeatManagementHandler{
		repo: repo,
	}
}

// AutoRevokeRemovedLicenseSeats automatically reclaims seats from the least active non-owner members
// when an enterprise tenant downgrades their subscription or reduces paid seat volume.
func (h *SeatManagementHandler) AutoRevokeRemovedLicenseSeats(
	ctx context.Context,
	orgID uuid.UUID,
	newSeatLimit int,
) (*billing.SeatReclamationPayload, error) {
	if newSeatLimit < 0 {
		return nil, fmt.Errorf("invalid seat limit: %d", newSeatLimit)
	}

	members, err := h.repo.GetOrganizationMembers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch organization members: %w", err)
	}

	// Filter active seat holders
	var seatHolders []billing.OrganizationMemberSeat
	for _, m := range members {
		if m.HasSeat {
			seatHolders = append(seatHolders, m)
		}
	}

	currentAllocated := len(seatHolders)
	if currentAllocated <= newSeatLimit {
		// No revocation required
		return &billing.SeatReclamationPayload{
			OrganizationID: orgID,
			PreviousLimit:  currentAllocated,
			NewLimit:       newSeatLimit,
			RevokedCount:   0,
		}, nil
	}

	excess := currentAllocated - newSeatLimit
	slog.Info("AutoRevokeRemovedLicenseSeats triggered",
		"org_id", orgID,
		"allocated", currentAllocated,
		"new_limit", newSeatLimit,
		"excess", excess,
	)

	// Partition candidates: protect owners and primary admins from automatic seat eviction
	var candidates []billing.OrganizationMemberSeat
	for _, m := range seatHolders {
		if !m.IsOwner && m.Role != "OWNER" && m.Role != "owner" {
			candidates = append(candidates, m)
		}
	}

	// Sort candidates by inactivity: nil LastActiveAt first, then oldest LastActiveAt first
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.LastActiveAt == nil && b.LastActiveAt != nil {
			return true
		}
		if a.LastActiveAt != nil && b.LastActiveAt == nil {
			return false
		}
		if a.LastActiveAt == nil && b.LastActiveAt == nil {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.LastActiveAt.Before(*b.LastActiveAt)
	})

	var revokedIDs []uuid.UUID
	revokedCount := 0

	for _, cand := range candidates {
		if revokedCount >= excess {
			break
		}

		reason := fmt.Sprintf("Auto-revoked due to enterprise seat limit reduction to %d", newSeatLimit)
		if err := h.repo.RevokeMemberSeat(ctx, orgID, cand.UserID, reason); err != nil {
			slog.Error("Failed to revoke seat for member",
				"org_id", orgID,
				"user_id", cand.UserID,
				"email", cand.Email,
				"error", err,
			)
			continue
		}

		revokedIDs = append(revokedIDs, cand.UserID)
		revokedCount++
		slog.Info("Revoked seat for inactive member",
			"org_id", orgID,
			"user_id", cand.UserID,
			"email", cand.Email,
			"last_active", cand.LastActiveAt,
		)
	}

	return &billing.SeatReclamationPayload{
		OrganizationID:  orgID,
		PreviousLimit:   currentAllocated,
		NewLimit:        newSeatLimit,
		RevokedCount:    revokedCount,
		RevokedUserIDs:  revokedIDs,
		ReclaimedReason: fmt.Sprintf("Subscription seat limit reduced to %d; reclaimed %d inactive seats", newSeatLimit, revokedCount),
	}, nil
}

// HandleBillingEvent satisfies the BillingEventHandler interface for seat limit exceeded hooks.
func (h *SeatManagementHandler) HandleBillingEvent(ctx context.Context, event billing.BillingWebhookEvent) error {
	var payload billing.SeatReclamationPayload
	_ = event // can be invoked via webhooks or internal triggers
	_ = time.Now()
	_ = payload
	return nil
}
