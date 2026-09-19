package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/recipient"
)

// OutboxPublisher represents an outbox relay or message queue producer for transactional dispatch.
type OutboxPublisher interface {
	PublishOutbox(ctx context.Context, exchange string, routingKey string, payload []byte) error
}

// NotificationService provides the high-level application entry point for emitting notifications across domains.
type NotificationService struct {
	dispatcher *NotificationDispatcherService
	publisher  OutboxPublisher
}

// NewNotificationService creates a notification service instance.
func NewNotificationService(dispatcher *NotificationDispatcherService, publisher OutboxPublisher) *NotificationService {
	return &NotificationService{
		dispatcher: dispatcher,
		publisher:  publisher,
	}
}

// EmitInput defines strongly-typed parameters for triggering a notification event.
type EmitInput struct {
	Event          catalog.Event
	OrganizationID string
	Payload        interface{}
	Recipients     []recipient.Recipient
	CorrelationID  string
}

// Emit sends a notification event through the outbox relay or directly executes local dispatch.
func (s *NotificationService) Emit(
	ctx context.Context,
	event catalog.Event,
	orgID string,
	payload interface{},
	recipients []recipient.Recipient,
	correlationID string,
) error {
	defaults, exists := catalog.EventDefaultsMap[event]
	if !exists {
		return fmt.Errorf("unregistered notification event: %s", event)
	}

	// Audience-based events can omit explicit recipients
	if len(recipients) == 0 && len(defaults.DefaultRoles) == 0 {
		return fmt.Errorf("notification event %s requires at least one recipient", event)
	}

	if correlationID == "" {
		correlationID = fmt.Sprintf("scandrix-notif-%s-%d", uuid.New().String()[:8], time.Now().UnixNano())
	}

	// Convert payload to map[string]interface{}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize notification payload: %w", err)
	}
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		return fmt.Errorf("failed to deserialize payload to map: %w", err)
	}

	msg := NotificationMessage{
		Event:          event,
		Payload:        payloadMap,
		OrganizationID: orgID,
		Recipients:     recipients,
		CorrelationID:  correlationID,
	}

	if s.publisher != nil {
		msgBytes, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to serialize notification message: %w", err)
		}
		exchange := "notification.exchange"
		routingKey := fmt.Sprintf("notification.%s", event)
		return s.publisher.PublishOutbox(ctx, exchange, routingKey, msgBytes)
	}

	if s.dispatcher != nil {
		return s.dispatcher.Dispatch(ctx, msg)
	}

	return nil
}
