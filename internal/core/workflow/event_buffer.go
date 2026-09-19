package workflow

import (
	"fmt"
	"sync"
	"time"
)

// DefaultEventBufferTTL matches DEFAULT_TTL from ScanDrix (5 minutes).
const DefaultEventBufferTTL = 5 * time.Minute

// StageCompletedEvent mirrors ScanDrix StageCompletedEvent interface.
type StageCompletedEvent struct {
	EventType string         `json:"eventType"`
	EventKey  string         `json:"eventKey"`
	TaskID    string         `json:"taskId"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Duration  int64          `json:"duration,omitempty"`
}

type eventBufferEntry struct {
	event     StageCompletedEvent
	timestamp time.Time
	ttl       time.Duration
}

// EventBufferService mirrors ScanDrix EventBufferService: in-memory buffer with TTL to prevent race conditions.
// Stores events that arrive before workflow is paused.
type EventBufferService struct {
	mu     sync.Mutex
	buffer map[string]eventBufferEntry
}

// NewEventBufferService instantiates an event buffer service.
func NewEventBufferService() *EventBufferService {
	return &EventBufferService{
		buffer: make(map[string]eventBufferEntry),
	}
}

func (b *EventBufferService) getKey(eventType, eventKey string) string {
	return fmt.Sprintf("%s:%s", eventType, eventKey)
}

// Store stores an event in buffer with TTL.
func (b *EventBufferService) Store(eventType, eventKey string, event StageCompletedEvent, ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultEventBufferTTL
	}
	key := b.getKey(eventType, eventKey)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer[key] = eventBufferEntry{
		event:     event,
		timestamp: time.Now(),
		ttl:       ttl,
	}
}

// Check checks if an event exists in buffer and is not expired. Consumes and removes it if found.
func (b *EventBufferService) Check(eventType, eventKey string) (*StageCompletedEvent, bool) {
	key := b.getKey(eventType, eventKey)

	b.mu.Lock()
	defer b.mu.Unlock()

	entry, exists := b.buffer[key]
	if !exists {
		return nil, false
	}

	if time.Since(entry.timestamp) > entry.ttl {
		delete(b.buffer, key)
		return nil, false
	}

	delete(b.buffer, key)
	return &entry.event, true
}

// Cleanup removes expired entries.
func (b *EventBufferService) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	for k, v := range b.buffer {
		if now.Sub(v.timestamp) > v.ttl {
			delete(b.buffer, k)
		}
	}
}
