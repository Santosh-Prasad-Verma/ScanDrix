// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ProviderType identifies the external payment processor or billing gateway.
type ProviderType string

const (
	ProviderStripe   ProviderType = "stripe"
	ProviderRazorpay ProviderType = "razorpay"
	ProviderManual   ProviderType = "manual"
)

// DunningState represents the current state of an organization's payment delinquency lifecycle.
type DunningState string

const (
	DunningStateGoodStanding DunningState = "good_standing"
	DunningStateWarning      DunningState = "warning"
	DunningStateRestricted   DunningState = "restricted"
	DunningStateSuspended    DunningState = "suspended"
)

// QuotaAllocation defines runtime limits assigned according to plan tier.
type QuotaAllocation struct {
	PlanTier             string   `json:"plan_tier"`
	MonthlyTokenBudget   int64    `json:"monthly_token_budget"`
	ConcurrentReviewJobs int      `json:"concurrent_review_jobs"`
	CustomRulesAllowed   int      `json:"custom_rules_allowed"`
	MaxSeats             int      `json:"max_seats"`
	AllowedFeatures      []string `json:"allowed_features"`
}

// OrganizationMemberSeat represents an organization member's seat allocation status.
type OrganizationMemberSeat struct {
	UserID       uuid.UUID  `json:"user_id"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	IsOwner      bool       `json:"is_owner"`
	HasSeat      bool       `json:"has_seat"`
	LastActiveAt *time.Time `json:"last_active_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// IBillingRepository abstracts persistence for billing, licenses, quotas, and audit trails.
type IBillingRepository interface {
	GetOrganizationLicense(ctx context.Context, orgID uuid.UUID) (*LicenseInfo, error)
	UpdateOrganizationPlan(ctx context.Context, orgID uuid.UUID, newTier string, seatLimit int, features []string) error
	GetOrganizationMembers(ctx context.Context, orgID uuid.UUID) ([]OrganizationMemberSeat, error)
	RevokeMemberSeat(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, reason string) error
	RecordBillingTransaction(ctx context.Context, tx BillingTransactionRecord) error
	UpdateDunningState(ctx context.Context, orgID uuid.UUID, state DunningState, gracePeriodEnd *time.Time) error
	GetDunningState(ctx context.Context, orgID uuid.UUID) (DunningState, *time.Time, error)
	ResetMonthlyTokenUsage(ctx context.Context, orgID uuid.UUID) error
	RecordFailedPayment(ctx context.Context, failure PaymentFailureRecord) error
}

// LicenseInfo holds active entitlement and limits for an organization.
type LicenseInfo struct {
	OrganizationID     uuid.UUID       `json:"organization_id"`
	PlanTier           string          `json:"plan_tier"`
	TotalSeats         int             `json:"total_seats"`
	AllocatedSeats     int             `json:"allocated_seats"`
	ExpiresAt          time.Time       `json:"expires_at"`
	Quota              QuotaAllocation `json:"quota"`
	Features           []string        `json:"features"`
	SubscriptionStatus string          `json:"subscription_status"`
	Provider           ProviderType    `json:"provider"`
	CustomerID         string          `json:"customer_id"`
	SubscriptionID     string          `json:"subscription_id"`
}

// BillingTransactionRecord captures financial transaction audits.
type BillingTransactionRecord struct {
	ID             uuid.UUID    `json:"id"`
	OrganizationID uuid.UUID    `json:"organization_id"`
	Provider       ProviderType `json:"provider"`
	TransactionID  string       `json:"transaction_id"`
	InvoiceID      string       `json:"invoice_id"`
	Amount         int64        `json:"amount"` // in minor units (e.g. cents)
	Currency       string       `json:"currency"`
	Status         string       `json:"status"`
	PlanTier       string       `json:"plan_tier"`
	Timestamp      time.Time    `json:"timestamp"`
}

// PaymentFailureRecord tracks payment failures for dunning workflows.
type PaymentFailureRecord struct {
	OrganizationID uuid.UUID    `json:"organization_id"`
	Provider       ProviderType `json:"provider"`
	InvoiceID      string       `json:"invoice_id"`
	Amount         int64        `json:"amount"`
	Currency       string       `json:"currency"`
	FailureReason  string       `json:"failure_reason"`
	AttemptCount   int          `json:"attempt_count"`
	NextRetryAt    *time.Time   `json:"next_retry_at"`
	FailedAt       time.Time    `json:"failed_at"`
}
