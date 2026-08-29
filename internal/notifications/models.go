package notifications

import (
	"time"

	"github.com/google/uuid"
)

// ChannelType represents an alert delivery medium.
type ChannelType string

const (
	ChannelSlack ChannelType = "slack"
	ChannelTeams ChannelType = "teams"
	ChannelEmail ChannelType = "email"
	ChannelInApp ChannelType = "in_app"
)

// AlertCriticality categorizes the urgency of a notification.
type AlertCriticality string

const (
	CriticalityLow      AlertCriticality = "low"
	CriticalityMedium   AlertCriticality = "medium"
	CriticalityHigh     AlertCriticality = "high"
	CriticalityCritical AlertCriticality = "critical"
)

// DeliveryStatus tracks dispatch outcomes.
type DeliveryStatus string

const (
	DeliveryPending     DeliveryStatus = "pending"
	DeliverySent        DeliveryStatus = "sent"
	DeliveryFailed      DeliveryStatus = "failed"
	DeliveryRateLimited DeliveryStatus = "rate_limited"
)

// NotificationEvent describes an event ready for delivery.
type NotificationEvent struct {
	ID           uuid.UUID        `json:"id"`
	WorkspaceID  uuid.UUID        `json:"workspace_id"`
	RepositoryID uuid.UUID        `json:"repository_id"`
	EventType    string           `json:"event_type"` // review_completed, critical_vulnerability, byok_auth_failed
	Criticality  AlertCriticality `json:"criticality"`
	Title        string           `json:"title"`
	Message      string           `json:"message"`
	TargetURL    string           `json:"target_url,omitempty"`
	Recipient    string           `json:"recipient"` // email, slack webhook, channel id
	Metadata     map[string]any   `json:"metadata,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
}

// DeliveryRecord tracks the execution audit of a notification delivery.
type DeliveryRecord struct {
	ID        uuid.UUID      `json:"id"`
	EventID   uuid.UUID      `json:"event_id"`
	Channel   ChannelType    `json:"channel"`
	Recipient string         `json:"recipient"`
	Status    DeliveryStatus `json:"status"`
	ErrorMsg  string         `json:"error_msg,omitempty"`
	SentAt    time.Time      `json:"sent_at"`
}

// NotificationRoutingRule configures which channels receive specific event categories.
type NotificationRoutingRule struct {
	ID             uuid.UUID        `json:"id"`
	WorkspaceID    uuid.UUID        `json:"workspace_id"`
	MinCriticality AlertCriticality `json:"min_criticality"`
	Channels       []ChannelType    `json:"channels"`
	WebhookURL     string           `json:"webhook_url,omitempty"`
	Enabled        bool             `json:"enabled"`
}
