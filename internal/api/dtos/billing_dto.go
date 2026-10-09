package dtos

import "time"

// CreateOrderDTO specifies parameters to initiate a Razorpay order.
type CreateOrderDTO struct {
	Plan     string `json:"plan"`     // "PRO", "TEAM", "ENTERPRISE"
	Currency string `json:"currency"` // "INR", "USD"
	// BillingInterval is the subscription term: "monthly" or "annual".
	//
	// This was absent, so an annual selection was charged at the monthly price
	// with no error anywhere. Empty is treated as monthly.
	BillingInterval string `json:"billing_interval"` // "monthly", "annual"
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

// CalculateDowngradeDTO specifies target downgrade tier.
type CalculateDowngradeDTO struct {
	TargetPlan string `json:"target_plan"` // "COMMUNITY", "DEVELOPER", "TEAM"
	Currency   string `json:"currency,omitempty"`
}

// ProrationResultDTO returns itemized breakdown of downgrade credits.
type ProrationResultDTO struct {
	CurrentPlan          string    `json:"current_plan"`
	TargetPlan           string    `json:"target_plan"`
	Currency             string    `json:"currency"`
	CurrentPlanRate      float64   `json:"current_plan_rate"`
	TargetPlanRate       float64   `json:"target_plan_rate"`
	TotalCycleDays       int       `json:"total_cycle_days"`
	DaysRemaining        int       `json:"days_remaining"`
	ProratedCreditAmount float64   `json:"prorated_credit_amount"`
	CreditBalanceApplied float64   `json:"credit_balance_applied"`
	NextBillingDate      time.Time `json:"next_billing_date"`
	ImmediateEffect      bool      `json:"immediate_effect"`
}

// ConfirmDowngradeDTO executes the automated self-service plan downgrade.
type ConfirmDowngradeDTO struct {
	TargetPlan      string `json:"target_plan"`
	ImmediateEffect bool   `json:"immediate_effect"`
	Reason          string `json:"reason,omitempty"`
}

// ConfirmDowngradeResponse reports the outcome of the automated downgrade.
type ConfirmDowngradeResponse struct {
	Success              bool      `json:"success"`
	NewPlanTier          string    `json:"new_plan_tier"`
	CreditBalanceAdded   float64   `json:"credit_balance_added"`
	Currency             string    `json:"currency"`
	Message              string    `json:"message"`
	EffectiveAt          time.Time `json:"effective_at"`
	NextBillingDate      time.Time `json:"next_billing_date"`
	NewTotalSeats        int       `json:"new_total_seats"`
	NewMonthlyTokenLimit int64     `json:"new_monthly_token_limit"`
}

