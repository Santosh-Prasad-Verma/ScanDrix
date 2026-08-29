package streaming

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// StreamBroker manages topic-based real-time pub/sub distribution for active reviews.
type StreamBroker struct {
	mu     sync.RWMutex
	topics map[uuid.UUID]map[string]*ClientSubscription // reviewID -> clientID -> subscription
}

// NewStreamBroker initializes the streaming pub/sub broker.
func NewStreamBroker() *StreamBroker {
	return &StreamBroker{
		topics: make(map[uuid.UUID]map[string]*ClientSubscription),
	}
}

// Subscribe registers a client to receive events for a specific review ID.
func (b *StreamBroker) Subscribe(reviewID uuid.UUID, clientID string) *ClientSubscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, exists := b.topics[reviewID]
	if !exists {
		subs = make(map[string]*ClientSubscription)
		b.topics[reviewID] = subs
	}

	sub := &ClientSubscription{
		ClientID:   clientID,
		ReviewID:   reviewID,
		EventChan:  make(chan StreamEvent, 64),
		ClosedChan: make(chan struct{}),
	}

	subs[clientID] = sub
	return sub
}

// Unsubscribe removes a client subscription and releases channels.
func (b *StreamBroker) Unsubscribe(sub *ClientSubscription) {
	if sub == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	subs, exists := b.topics[sub.ReviewID]
	if !exists {
		return
	}

	if _, ok := subs[sub.ClientID]; ok {
		delete(subs, sub.ClientID)
		close(sub.ClosedChan)
		close(sub.EventChan)
	}

	if len(subs) == 0 {
		delete(b.topics, sub.ReviewID)
	}
}

// Broadcast distributes an event to all subscribers listening to event.ReviewID.
// Non-blocking dispatch ensures slow clients never block the pipeline execution.
func (b *StreamBroker) Broadcast(event StreamEvent) int {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	b.mu.RLock()
	subs, exists := b.topics[event.ReviewID]
	if !exists || len(subs) == 0 {
		b.mu.RUnlock()
		return 0
	}

	delivered := 0
	for _, sub := range subs {
		select {
		case sub.EventChan <- event:
			delivered++
		default:
			// Buffer full: drop non-blocking to protect broker stability
		}
	}
	b.mu.RUnlock()

	return delivered
}

// SubscriberCount returns the number of active listeners for a review session.
func (b *StreamBroker) SubscriberCount(reviewID uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.topics[reviewID]
	if !exists {
		return 0
	}
	return len(subs)
}
