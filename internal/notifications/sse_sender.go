package notifications

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/streaming"
)

// InAppSSESender distributes real-time in-app notifications over the active SSE StreamBroker.
type InAppSSESender struct {
	broker *streaming.StreamBroker
}

// NewInAppSSESender initializes the SSE in-app channel sender.
func NewInAppSSESender(broker *streaming.StreamBroker) *InAppSSESender {
	return &InAppSSESender{broker: broker}
}

// Send broadcasts the alert event to active SSE clients.
func (s *InAppSSESender) Send(ctx context.Context, event NotificationEvent, recipient string) error {
	if s.broker == nil {
		return nil
	}

	streamEvt := streaming.StreamEvent{
		ID:       uuid.New(),
		ReviewID: event.RepositoryID, // Channel/Topic key
		Type:     streaming.EventReviewCompleted,
		Payload: map[string]any{
			"notification_id": event.ID,
			"event_type":      event.EventType,
			"title":           event.Title,
			"message":         event.Message,
			"criticality":     event.Criticality,
			"recipient":       recipient,
		},
		Timestamp: time.Now().UTC(),
	}

	s.broker.Broadcast(streamEvt)
	return nil
}
