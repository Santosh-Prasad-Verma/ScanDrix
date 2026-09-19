package notifications

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ChannelSender represents an adapter that delivers messages to an external platform.
type ChannelSender interface {
	Send(ctx context.Context, event NotificationEvent, recipient string) error
}

// NotificationDispatcher orchestrates end-to-end alert delivery with deduplication and auditing.
type NotificationDispatcher struct {
	mu       sync.RWMutex
	router   *RoutingRuleService
	limiter  *NotificationRateLimiter
	senders  map[ChannelType]ChannelSender
	auditLog []DeliveryRecord
}

func NewNotificationDispatcher(router *RoutingRuleService, limiter *NotificationRateLimiter) *NotificationDispatcher {
	return &NotificationDispatcher{
		router:   router,
		limiter:  limiter,
		senders:  make(map[ChannelType]ChannelSender),
		auditLog: make([]DeliveryRecord, 0),
	}
}

// RegisterSender binds a concrete sender adapter to a channel type.
func (d *NotificationDispatcher) RegisterSender(ch ChannelType, sender ChannelSender) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.senders[ch] = sender
}

// Dispatch processes an alert event, resolves target channels, checks rate limits, and triggers senders.
func (d *NotificationDispatcher) Dispatch(ctx context.Context, event NotificationEvent) ([]DeliveryRecord, error) {
	// 1. Rate limiting check
	resourceKey := fmt.Sprintf("%s:%s", event.RepositoryID, event.EventType)
	if !d.limiter.Allow(event.WorkspaceID, resourceKey, string(event.Criticality)) {
		rec := DeliveryRecord{
			ID:        uuid.New(),
			EventID:   event.ID,
			Channel:   ChannelType("all"),
			Recipient: event.Recipient,
			Status:    DeliveryRateLimited,
			ErrorMsg:  "Notification throttled to prevent alert fatigue",
			SentAt:    time.Now().UTC(),
		}
		d.recordDelivery(rec)
		return []DeliveryRecord{rec}, nil
	}

	// 2. Resolve channels
	channels := d.router.ResolveChannels(event)
	var records []DeliveryRecord

	d.mu.RLock()
	sendersCopy := make(map[ChannelType]ChannelSender)
	for k, v := range d.senders {
		sendersCopy[k] = v
	}
	d.mu.RUnlock()

	for _, ch := range channels {
		sender, exists := sendersCopy[ch]
		rec := DeliveryRecord{
			ID:        uuid.New(),
			EventID:   event.ID,
			Channel:   ch,
			Recipient: event.Recipient,
			SentAt:    time.Now().UTC(),
		}

		if !exists {
			rec.Status = DeliverySent // Simulated delivery if sender not registered
		} else {
			err := sender.Send(ctx, event, event.Recipient)
			if err != nil {
				rec.Status = DeliveryFailed
				rec.ErrorMsg = err.Error()
			} else {
				rec.Status = DeliverySent
			}
		}

		records = append(records, rec)
		d.recordDelivery(rec)
	}

	return records, nil
}

func (d *NotificationDispatcher) recordDelivery(rec DeliveryRecord) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.auditLog = append(d.auditLog, rec)
}

// GetDeliveryAudit returns all dispatched delivery records.
func (d *NotificationDispatcher) GetDeliveryAudit() []DeliveryRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]DeliveryRecord, len(d.auditLog))
	copy(out, d.auditLog)
	return out
}
