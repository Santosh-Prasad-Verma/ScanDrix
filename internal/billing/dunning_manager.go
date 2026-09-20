// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DunningConfig defines timing and thresholds for payment failure escalations.
type DunningConfig struct {
	WarningGracePeriod    time.Duration `json:"warning_grace_period"`    // e.g. 7 days
	RestrictedGracePeriod time.Duration `json:"restricted_grace_period"` // e.g. 7 days after warning
	MaxFailureAttempts    int           `json:"max_failure_attempts"`    // e.g. 4 attempts
}

// DefaultDunningConfig returns production-grade timeline defaults.
func DefaultDunningConfig() DunningConfig {
	return DunningConfig{
		WarningGracePeriod:    7 * 24 * time.Hour,
		RestrictedGracePeriod: 7 * 24 * time.Hour,
		MaxFailureAttempts:    4,
	}
}

// DunningManager governs the delinquency state machine for enterprise tenants.
type DunningManager struct {
	mu     sync.RWMutex
	repo   IBillingRepository
	config DunningConfig
}

// NewDunningManager constructs a new dunning state controller.
func NewDunningManager(repo IBillingRepository, config DunningConfig) *DunningManager {
	return &DunningManager{
		repo:   repo,
		config: config,
	}
}

// HandlePaymentFailure processes a newly registered payment failure.
func (m *DunningManager) HandlePaymentFailure(ctx context.Context, payload PaymentFailurePayload) (*DunningStateChangedPayload, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentState, graceEnd, err := m.repo.GetDunningState(ctx, payload.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current dunning state: %w", err)
	}

	now := time.Now().UTC()
	var newState DunningState
	var newGraceEnd *time.Time
	var reason string

	switch currentState {
	case DunningStateGoodStanding, "":
		// First payment failure: transition to warning with grace period
		newState = DunningStateWarning
		end := now.Add(m.config.WarningGracePeriod)
		newGraceEnd = &end
		reason = fmt.Sprintf("Payment failure attempt #%d for invoice %s: %s", payload.AttemptCount, payload.InvoiceID, payload.FailureReason)

	case DunningStateWarning:
		if (graceEnd != nil && now.After(*graceEnd)) || payload.AttemptCount >= m.config.MaxFailureAttempts {
			// Grace period expired or repeated failures: escalate to restricted
			newState = DunningStateRestricted
			end := now.Add(m.config.RestrictedGracePeriod)
			newGraceEnd = &end
			reason = fmt.Sprintf("Warning grace period elapsed; escalating to restricted state (attempts: %d)", payload.AttemptCount)
		} else {
			// Still in warning period, keep existing state and grace end
			newState = DunningStateWarning
			newGraceEnd = graceEnd
			reason = fmt.Sprintf("Repeated failure #%d during active warning grace period", payload.AttemptCount)
		}

	case DunningStateRestricted:
		if graceEnd != nil && now.After(*graceEnd) {
			// Restricted period expired without payment: suspend organization review operations
			newState = DunningStateSuspended
			newGraceEnd = nil
			reason = "Final dunning grace period elapsed without settlement; organization suspended"
		} else {
			newState = DunningStateRestricted
			newGraceEnd = graceEnd
			reason = fmt.Sprintf("Repeated failure #%d in restricted state", payload.AttemptCount)
		}

	case DunningStateSuspended:
		newState = DunningStateSuspended
		newGraceEnd = nil
		reason = fmt.Sprintf("Payment failure #%d on already suspended tenant", payload.AttemptCount)
	}

	if err := m.repo.UpdateDunningState(ctx, payload.OrganizationID, newState, newGraceEnd); err != nil {
		return nil, fmt.Errorf("failed to persist dunning state transition: %w", err)
	}

	return &DunningStateChangedPayload{
		OrganizationID: payload.OrganizationID,
		PreviousState:  currentState,
		NewState:       newState,
		GracePeriodEnd: newGraceEnd,
		Reason:         reason,
	}, nil
}

// HandlePaymentSuccess clears delinquency and returns the organization to Good Standing.
func (m *DunningManager) HandlePaymentSuccess(ctx context.Context, orgID uuid.UUID, invoiceID string) (*DunningStateChangedPayload, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentState, _, err := m.repo.GetDunningState(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current dunning state: %w", err)
	}

	if currentState == DunningStateGoodStanding {
		return nil, nil // Already in good standing, no transition needed
	}

	if err := m.repo.UpdateDunningState(ctx, orgID, DunningStateGoodStanding, nil); err != nil {
		return nil, fmt.Errorf("failed to restore organization to good standing: %w", err)
	}

	return &DunningStateChangedPayload{
		OrganizationID: orgID,
		PreviousState:  currentState,
		NewState:       DunningStateGoodStanding,
		GracePeriodEnd: nil,
		Reason:         fmt.Sprintf("Payment successfully settled for invoice %s", invoiceID),
	}, nil
}

// CheckDelinquencyRestrictions verifies if code review jobs or rule configurations are permitted.
func (m *DunningManager) CheckDelinquencyRestrictions(ctx context.Context, orgID uuid.UUID) (bool, string, error) {
	currentState, graceEnd, err := m.repo.GetDunningState(ctx, orgID)
	if err != nil {
		return false, "", err
	}

	now := time.Now().UTC()
	switch currentState {
	case DunningStateSuspended:
		return false, "Code review execution is suspended due to unpaid enterprise balance. Please settle outstanding invoices.", nil
	case DunningStateRestricted:
		if graceEnd != nil && now.After(*graceEnd) {
			// Auto escalate to suspended if inspected after grace period
			_ = m.repo.UpdateDunningState(ctx, orgID, DunningStateSuspended, nil)
			return false, "Code review execution is suspended due to expired billing grace period.", nil
		}
		return true, "Warning: Account is restricted due to overdue payment. Review concurrency is limited.", nil
	case DunningStateWarning:
		return true, "Notice: Payment failed. Please update payment method to avoid service interruption.", nil
	default:
		return true, "", nil
	}
}
