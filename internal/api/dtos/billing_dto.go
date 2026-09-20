package dtos

import "time"

// CreateOrderDTO specifies parameters to initiate a Razorpay order.
type CreateOrderDTO struct {
	Plan     string `json:"plan"`     // "PRO", "TEAM", "ENTERPRISE"
	Currency string `json:"currency"` // "INR", "USD"
}

// VerifyPaymentDTO passes frontend checkout cryptographic proof.
type VerifyPaymentDTO struct {
	OrderID        string `json:"razorpay_order_id"`
	PaymentID      string `json:"razorpay_payment_id"`
	Signature      string `json:"razorpay_signature"`
	PlanTier       string `json:"plan_tier"`
	RecipientEmail string `json:"recipient_email,omitempty"`
	RecipientName  string `json:"recipient_name,omitempty"`
}

// WorkspacePlanStatusResponse provides comprehensive plan entitlement and consumption metrics.
type WorkspacePlanStatusResponse struct {
	PlanTier          string    `json:"plan_tier"`
	OrganizationName  string    `json:"organization_name"`
	TotalSeats        int       `json:"total_seats"`
	AllocatedSeats    int       `json:"allocated_seats"`
	ExpiresAt         time.Time `json:"expires_at"`
	MonthlyTokenLimit int64     `json:"monthly_token_limit"`
	MonthlyTokensUsed int64     `json:"monthly_tokens_used"`
	BurstLimitPerMin  int64     `json:"burst_limit_per_min"`
	BurstTokensUsed   int64     `json:"burst_tokens_used"`
	AllocatedModels   []string  `json:"allocated_models"`
	FeaturesEnabled   []string  `json:"features_enabled"`
	BYOKAllowed       bool      `json:"byok_allowed"`
}
