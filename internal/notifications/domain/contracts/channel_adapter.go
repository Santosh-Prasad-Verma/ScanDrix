package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// DeliveryContext carries all contextual metadata needed by a channel adapter to execute delivery.
type DeliveryContext struct {
	DeliveryID     string                 `json:"deliveryId"`
	UserID         string                 `json:"userId,omitempty"`
	UserEmail      string                 `json:"userEmail,omitempty"`
	UserRole       string                 `json:"userRole,omitempty"`
	OrganizationID string                 `json:"organizationId"`
	Event          catalog.Event          `json:"event"`
	Criticality    enums.Criticality      `json:"criticality"`
	Title          string                 `json:"title"`
	Body           string                 `json:"body"`
	CtaURL         string                 `json:"ctaUrl,omitempty"`
	Category       string                 `json:"category"`
	Metadata       map[string]interface{} `json:"metadata"`
	CorrelationID  string                 `json:"correlationId"`
}

// ChannelAdapter defines the contract implemented by every notification channel provider.
type ChannelAdapter interface {
	Channel() enums.Channel
	Deliver(ctx context.Context, deliveryCtx DeliveryContext) error
}
