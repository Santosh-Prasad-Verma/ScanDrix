package entities

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// NotificationDelivery represents an individual channel delivery attempt and audit record.
type NotificationDelivery struct {
	UUID            uuid.UUID              `json:"uuid"`
	OrganizationID  uuid.UUID              `json:"organizationId"`
	RecipientUserID *uuid.UUID             `json:"recipientUserId,omitempty"`
	Event           string                 `json:"event"`
	Criticality     enums.Criticality      `json:"criticality"`
	Channel         enums.Channel          `json:"channel"`
	Title           string                 `json:"title"`
	Body            string                 `json:"body"`
	CtaURL          *string                `json:"ctaUrl,omitempty"`
	Category        string                 `json:"category"`
	RecipientEmail  *string                `json:"recipientEmail,omitempty"`
	RecipientRole   *string                `json:"recipientRole,omitempty"`
	DeliveryStatus  enums.DeliveryStatus   `json:"deliveryStatus"`
	Metadata        map[string]interface{} `json:"metadata"`
	CorrelationID   string                 `json:"correlationId"`
	LastError       *string                `json:"lastError,omitempty"`
	DeliveredAt     *time.Time             `json:"deliveredAt,omitempty"`
	Attempts        int                    `json:"attempts"`
	NextAttemptAt   *time.Time             `json:"nextAttemptAt,omitempty"`
	LockedAt        *time.Time             `json:"lockedAt,omitempty"`
	LockedBy        *string                `json:"lockedBy,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

// NewDelivery initializes a delivery record in pending status.
func NewDelivery(
	orgID uuid.UUID,
	event string,
	criticality enums.Criticality,
	channel enums.Channel,
	title string,
	body string,
	category string,
	correlationID string,
) *NotificationDelivery {
	now := time.Now().UTC()
	return &NotificationDelivery{
		UUID:           uuid.New(),
		OrganizationID: orgID,
		Event:          event,
		Criticality:    criticality,
		Channel:        channel,
		Title:          title,
		Body:           body,
		Category:       category,
		DeliveryStatus: enums.StatusPending,
		Metadata:       make(map[string]interface{}),
		CorrelationID:  correlationID,
		Attempts:       0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
