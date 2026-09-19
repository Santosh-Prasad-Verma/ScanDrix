package domain

import (
	"time"

	"github.com/google/uuid"
)

// CodeReviewExecution stores stage-by-stage pipeline telemetry.
type CodeReviewExecution struct {
	TenantScopedEntity
	ReviewID      uuid.UUID `json:"review_id" db:"review_id"`
	StageName     string    `json:"stage_name" db:"stage_name"`
	Status        string    `json:"status" db:"status"`
	DurationMs    int64     `json:"duration_ms" db:"duration_ms"`
	InputTokens   int       `json:"input_tokens" db:"input_tokens"`
	OutputTokens  int       `json:"output_tokens" db:"output_tokens"`
	ErrorMessage  *string   `json:"error_message,omitempty" db:"error_message"`
	StageMetadata JSONBMap  `json:"stage_metadata" db:"stage_metadata"`
}

// CodeReviewExecutionModel alias.
type CodeReviewExecutionModel = CodeReviewExecution

// TokenUsageRecord models granular token consumption and cost telemetry.
type TokenUsageRecord struct {
	BaseEntity
	WorkspaceID      uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	ReviewID         *uuid.UUID `json:"review_id,omitempty" db:"review_id"`
	RepositoryID     *uuid.UUID `json:"repository_id,omitempty" db:"repository_id"`
	ModelName        string     `json:"model_name" db:"model_name"`
	PromptTokens     int        `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens" db:"completion_tokens"`
	TotalTokens      int        `json:"total_tokens" db:"total_tokens"`
	EstimatedCostUSD float64    `json:"estimated_cost_usd" db:"estimated_cost_usd"`
	OperationType    string     `json:"operation_type" db:"operation_type"`
	RecordedAt       time.Time  `json:"recorded_at" db:"recorded_at"`
}

// TokenUsageRecordModel alias.
type TokenUsageRecordModel = TokenUsageRecord

// ObservabilityTelemetry stores aggregated trace stats and system performance telemetry.
type ObservabilityTelemetry struct {
	TenantScopedEntity
	ServiceName   string   `json:"service_name" db:"service_name"`
	TraceID       string   `json:"trace_id" db:"trace_id"`
	SpanID        string   `json:"span_id" db:"span_id"`
	OperationName string   `json:"operation_name" db:"operation_name"`
	DurationMs    int64    `json:"duration_ms" db:"duration_ms"`
	StatusCode    int      `json:"status_code" db:"status_code"`
	Attributes    JSONBMap `json:"attributes" db:"attributes"`
	Events        JSONBMap `json:"events" db:"events"`
}

// ObservabilityTelemetryModel alias.
type ObservabilityTelemetryModel = ObservabilityTelemetry

// NotificationDelivery tracks delivery status to Slack, Discord, Email, or Webhook.
type NotificationDelivery struct {
	TenantScopedEntity
	NotificationID uuid.UUID  `json:"notification_id" db:"notification_id"`
	Channel        string     `json:"channel" db:"channel"`
	Destination    string     `json:"destination" db:"destination"`
	Status         string     `json:"status" db:"status"`
	Attempts       int        `json:"attempts" db:"attempts"`
	ErrorMessage   *string    `json:"error_message,omitempty" db:"error_message"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
}

// NotificationDeliveryModel alias.
type NotificationDeliveryModel = NotificationDelivery

// UserNotification represents in-app notifications displayed on the developer dashboard.
type UserNotification struct {
	TenantScopedEntity
	UserID   uuid.UUID  `json:"user_id" db:"user_id"`
	Title    string     `json:"title" db:"title"`
	Message  string     `json:"message" db:"message"`
	Category string     `json:"category" db:"category"`
	LinkURL  *string    `json:"link_url,omitempty" db:"link_url"`
	IsRead   bool       `json:"is_read" db:"is_read"`
	ReadAt   *time.Time `json:"read_at,omitempty" db:"read_at"`
}

// UserNotificationModel alias.
type UserNotificationModel = UserNotification

// RoutingRule determines which team or developer receives alerts for specific repositories or finding types.
type RoutingRule struct {
	TenantScopedEntity
	TeamID            *uuid.UUID  `json:"team_id,omitempty" db:"team_id"`
	RepositoryID      *uuid.UUID  `json:"repository_id,omitempty" db:"repository_id"`
	SeverityFilter    StringSlice `json:"severity_filter" db:"severity_filter"`
	ChannelType       string      `json:"channel_type" db:"channel_type"`
	DestinationTarget string      `json:"destination_target" db:"destination_target"`
	IsActive          bool        `json:"is_active" db:"is_active"`
}

// RoutingRuleModel alias.
type RoutingRuleModel = RoutingRule

// Interaction records user engagement metrics with dashboard components and code suggestions.
type Interaction struct {
	TenantScopedEntity
	UserID          uuid.UUID `json:"user_id" db:"user_id"`
	InteractionType string    `json:"interaction_type" db:"interaction_type"`
	TargetResource  string    `json:"target_resource" db:"target_resource"`
	TargetID        string    `json:"target_id" db:"target_id"`
	Metadata        JSONBMap  `json:"metadata" db:"metadata"`
}

// InteractionModel alias.
type InteractionModel = Interaction

// AuditLogRecord tracks compliance and administrative changes across an organization.
type AuditLogRecord struct {
	TenantScopedEntity
	ActorUserID    uuid.UUID `json:"actor_user_id" db:"actor_user_id"`
	Action         string    `json:"action" db:"action"`
	TargetResource string    `json:"target_resource" db:"target_resource"`
	TargetID       string    `json:"target_id" db:"target_id"`
	IPAddress      string    `json:"ip_address" db:"ip_address"`
	UserAgent      string    `json:"user_agent" db:"user_agent"`
	PreviousState  JSONBMap  `json:"previous_state" db:"previous_state"`
	NewState       JSONBMap  `json:"new_state" db:"new_state"`
	Metadata       JSONBMap  `json:"metadata" db:"metadata"`
}
