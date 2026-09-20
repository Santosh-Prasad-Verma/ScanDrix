// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BillingEventType enumerates billing lifecycle events.
type BillingEventType string

const (
	EventPlanChanged            BillingEventType = "billing.plan_changed"
	EventSeatLimitExceeded      BillingEventType = "billing.seat_limit_exceeded"
	EventPaymentFailed          BillingEventType = "billing.payment_failed"
	EventTrialExpiring          BillingEventType = "billing.trial_expiring"
	EventSubscriptionCancelled  BillingEventType = "billing.subscription_cancelled"
	EventInvoiceSucceeded       BillingEventType = "billing.invoice_succeeded"
	EventDunningStateChanged    BillingEventType = "billing.dunning_state_changed"
	EventLicenseSeatsReclaimed  BillingEventType = "billing.license_seats_reclaimed"
)

// BillingWebhookEvent wraps an ingested webhook with normalized metadata.
type BillingWebhookEvent struct {
	ID             string           `json:"id"`
	Provider       ProviderType     `json:"provider"`
	EventType      BillingEventType `json:"event_type"`
	OrganizationID uuid.UUID        `json:"organization_id"`
	WorkspaceID    uuid.UUID        `json:"workspace_id,omitempty"`
	Timestamp      time.Time        `json:"timestamp"`
	RawPayload     json.RawMessage  `json:"raw_payload"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
}

// PlanChangePayload represents data delivered on subscription plan alterations.
type PlanChangePayload struct {
	OrganizationID     uuid.UUID `json:"organization_id"`
	WorkspaceID        uuid.UUID `json:"workspace_id,omitempty"`
	PreviousTier       string    `json:"previous_tier"`
	NewTier            string    `json:"new_tier"`
	NewSeatLimit       int       `json:"new_seat_limit"`
	MonthlyTokenBudget int64     `json:"monthly_token_budget"`
	EffectiveAt        time.Time `json:"effective_at"`
	Currency           string    `json:"currency"`
	Amount             int64     `json:"amount"`
	AutoPruneSeats     bool      `json:"auto_prune_seats"`
}

// PaymentFailurePayload represents data delivered when a renewal or invoice charge fails.
type PaymentFailurePayload struct {
	OrganizationID   uuid.UUID  `json:"organization_id"`
	WorkspaceID      uuid.UUID  `json:"workspace_id,omitempty"`
	InvoiceID        string     `json:"invoice_id"`
	Amount           int64      `json:"amount"`
	Currency         string     `json:"currency"`
	FailureReason    string     `json:"failure_reason"`
	AttemptCount     int        `json:"attempt_count"`
	NextRetryAt      *time.Time `json:"next_retry_at,omitempty"`
	UpdatePaymentURL string     `json:"update_payment_url"`
}

// TrialExpiringPayload represents notifications for an expiring trial tier.
type TrialExpiringPayload struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	WorkspaceID    uuid.UUID `json:"workspace_id,omitempty"`
	TrialEndsAt    time.Time `json:"trial_ends_at"`
	DaysRemaining  int       `json:"days_remaining"`
	UpgradeURL     string    `json:"upgrade_url"`
}

// SubscriptionCancelledPayload represents an explicit cancellation or failure to renew.
type SubscriptionCancelledPayload struct {
	OrganizationID        uuid.UUID `json:"organization_id"`
	WorkspaceID           uuid.UUID `json:"workspace_id,omitempty"`
	EffectiveDate         time.Time `json:"effective_date"`
	Reason                string    `json:"reason"`
	ImmediateGracePeriod  bool      `json:"immediate_grace_period"`
	GracePeriodEnd        time.Time `json:"grace_period_end"`
}

// InvoiceSucceededPayload represents a confirmed paid invoice.
type InvoiceSucceededPayload struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	WorkspaceID    uuid.UUID `json:"workspace_id,omitempty"`
	InvoiceID      string    `json:"invoice_id"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	ResetTokens    bool      `json:"reset_tokens"`
}

// SeatReclamationPayload documents automatic removal of excess seat licenses.
type SeatReclamationPayload struct {
	OrganizationID  uuid.UUID   `json:"organization_id"`
	PreviousLimit   int         `json:"previous_limit"`
	NewLimit        int         `json:"new_limit"`
	RevokedCount    int         `json:"revoked_count"`
	RevokedUserIDs  []uuid.UUID `json:"revoked_user_ids"`
	ReclaimedReason string      `json:"reclaimed_reason"`
}

// DunningStateChangedPayload documents state machine status transitions.
type DunningStateChangedPayload struct {
	OrganizationID uuid.UUID    `json:"organization_id"`
	PreviousState  DunningState `json:"previous_state"`
	NewState       DunningState `json:"new_state"`
	GracePeriodEnd *time.Time   `json:"grace_period_end,omitempty"`
	Reason         string       `json:"reason"`
}
