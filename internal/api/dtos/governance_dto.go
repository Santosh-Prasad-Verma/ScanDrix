package dtos

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// PermissionCheckRequest asks if a user can perform an action on a resource.
type PermissionCheckRequest struct {
	Permission string `json:"permission"`
	ResourceID string `json:"resource_id,omitempty"`
}

// PermissionCheckResponse returns authorization verdict.
type PermissionCheckResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// LicenseResponse details the active enterprise subscription.
type LicenseResponse struct {
	LicenseKey       string    `json:"license_key"`
	OrganizationName string    `json:"organization_name"`
	PlanTier         string    `json:"plan_tier"` // "ENTERPRISE", "TEAM", "PRO"
	TotalSeats       int       `json:"total_seats"`
	AllocatedSeats   int       `json:"allocated_seats"`
	ExpiresAt        time.Time `json:"expires_at"`
	IsAirGapped      bool      `json:"is_air_gapped"`
	FeaturesEnabled  []string  `json:"features_enabled"`
}

// ActivateLicenseRequest sends a new enterprise license token.
type ActivateLicenseRequest struct {
	LicenseKey string `json:"license_key"`
}

// WebhookHealthResponse details recent webhook delivery reliability.
type WebhookHealthResponse struct {
	TotalDelivered   int64     `json:"total_delivered"`
	SuccessRate      float64   `json:"success_rate"`
	AverageLatencyMs float64   `json:"average_latency_ms"`
	RecentFailures   int       `json:"recent_failures"`
	LastEventAt      time.Time `json:"last_event_at"`
}

// OutboxLagResponse details asynchronous queue backlog.
type OutboxLagResponse struct {
	PendingCount    int       `json:"pending_count"`
	RetryingCount   int       `json:"retrying_count"`
	DeadLetterCount int       `json:"dead_letter_count"`
	OldestPendingAt time.Time `json:"oldest_pending_at"`
}

// NotificationChannelDTO defines alert destinations.
type NotificationChannelDTO struct {
	ID       uuid.UUID              `json:"id"`
	Type     string                 `json:"type"`   // "SLACK", "TEAMS", "EMAIL"
	Target   string                 `json:"target"` // URL or email
	Severity models.FindingSeverity `json:"severity_filter"`
	Enabled  bool                   `json:"enabled"`
}

// FindingFeedbackRequest records developer sentiment on review findings.
type FindingFeedbackRequest struct {
	FindingID uuid.UUID `json:"finding_id"`
	Sentiment string    `json:"sentiment"` // "HELPFUL", "FALSE_POSITIVE", "IRRELEVANT"
	Comments  string    `json:"comments,omitempty"`
}
