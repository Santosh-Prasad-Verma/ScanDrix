package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/entities"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// UserNotificationWithDelivery represents a user in-app notification joined with delivery metadata.
type UserNotificationWithDelivery struct {
	UUID       uuid.UUID              `json:"uuid"`
	UserID     uuid.UUID              `json:"userId"`
	DeliveryID uuid.UUID              `json:"deliveryId"`
	ReadAt     *time.Time             `json:"readAt,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
	Delivery   DeliverySummary        `json:"delivery"`
}

// DeliverySummary contains display fields from the parent delivery.
type DeliverySummary struct {
	UUID        uuid.UUID              `json:"uuid"`
	Event       string                 `json:"event"`
	Criticality enums.Criticality      `json:"criticality"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	CtaURL      *string                `json:"ctaUrl,omitempty"`
	Category    string                 `json:"category"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
}

// NotificationDeliveryRepository specifies persistence for notification delivery audit records.
type NotificationDeliveryRepository interface {
	Create(ctx context.Context, delivery *entities.NotificationDelivery) error
	UpdateStatus(ctx context.Context, deliveryID uuid.UUID, status enums.DeliveryStatus, errorMsg *string) error
	ScheduleRetry(ctx context.Context, deliveryID uuid.UUID, nextAttemptAt time.Time, errorMsg string) error
	ClaimRetryBatch(ctx context.Context, limit int, lockedBy string) ([]*entities.NotificationDelivery, error)
	FindByCorrelationID(ctx context.Context, correlationID string) ([]*entities.NotificationDelivery, error)
	FindByID(ctx context.Context, id uuid.UUID) (*entities.NotificationDelivery, error)
	FindByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*entities.NotificationDelivery, int, error)
}

// RoutingRuleRepository manages notification routing matrix configuration per organization.
type RoutingRuleRepository interface {
	FindByOrganization(ctx context.Context, orgID uuid.UUID) ([]*entities.RoutingRule, error)
	Resolve(ctx context.Context, orgID uuid.UUID, event string, role string) (*entities.RoutingRule, error)
	Upsert(ctx context.Context, rule *entities.RoutingRule) error
	UpsertBatch(ctx context.Context, rules []*entities.RoutingRule) error
	DeleteByOrganization(ctx context.Context, orgID uuid.UUID) (int64, error)
	DeleteByOrgEventRole(ctx context.Context, orgID uuid.UUID, event string, role string) (int64, error)
}

// UserNotificationRepository manages user-facing in-app notification rows.
type UserNotificationRepository interface {
	Create(ctx context.Context, un *entities.UserNotification) error
	FindByUser(ctx context.Context, userID uuid.UUID, limit, offset int, unreadOnly bool) ([]*UserNotificationWithDelivery, int, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkAsRead(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) error
	MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int, error)
}
