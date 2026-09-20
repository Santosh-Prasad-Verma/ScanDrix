// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package lifecycle

import (
	"sync"
	"time"
)

// EventType defines lifecycle event categories.
type EventType string

const (
	EventTurnStarted      EventType = "turn.started"
	EventTurnCompleted    EventType = "turn.completed"
	EventSessionStarted   EventType = "session.started"
	EventSessionCompleted EventType = "session.completed"
	EventHookInvoked      EventType = "hook.invoked"
	EventReviewTriggered  EventType = "review.triggered"
)

// LifecycleEvent represents an event dispatched during CLI execution.
type LifecycleEvent struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	SessionID string         `json:"session_id,omitempty"`
	TurnID    string         `json:"turn_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// EventListener is a callback function invoked when a lifecycle event is emitted.
type EventListener func(event LifecycleEvent)

// EventBus provides thread-safe pub/sub dispatching for CLI lifecycle events.
type EventBus struct {
	mu        sync.RWMutex
	listeners map[EventType][]EventListener
	history   []LifecycleEvent
	maxHistory int
}

// NewEventBus creates a new EventBus with bounded history.
func NewEventBus(maxHistory ...int) *EventBus {
	limit := 1000
	if len(maxHistory) > 0 && maxHistory[0] > 0 {
		limit = maxHistory[0]
	}
	return &EventBus{
		listeners:  make(map[EventType][]EventListener),
		history:    make([]LifecycleEvent, 0, limit),
		maxHistory: limit,
	}
}

// Subscribe registers a listener for a specific event type.
func (b *EventBus) Subscribe(eventType EventType, listener EventListener) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.listeners[eventType] = append(b.listeners[eventType], listener)

	// Return unsubscribe func
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		list := b.listeners[eventType]
		for i, l := range list {
			// Compare function pointers
			if &l == &listener {
				b.listeners[eventType] = append(list[:i], list[i+1:]...)
				break
			}
		}
	}
}

// Emit broadcasts an event to all subscribers and records it in history.
func (b *EventBus) Emit(event LifecycleEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	b.mu.Lock()
	if len(b.history) >= b.maxHistory {
		b.history = b.history[1:]
	}
	b.history = append(b.history, event)

	// Copy listeners to avoid holding lock during callback execution
	targets := make([]EventListener, len(b.listeners[event.Type]))
	copy(targets, b.listeners[event.Type])
	b.mu.Unlock()

	for _, listener := range targets {
		listener(event)
	}
}

// History returns a snapshot of recorded lifecycle events.
func (b *EventBus) History() []LifecycleEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	res := make([]LifecycleEvent, len(b.history))
	copy(res, b.history)
	return res
}

// FilterHistory returns recorded events matching a specific type.
func (b *EventBus) FilterHistory(eventType EventType) []LifecycleEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var filtered []LifecycleEvent
	for _, e := range b.history {
		if e.Type == eventType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// ClearHistory resets the recorded events.
func (b *EventBus) ClearHistory() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = b.history[:0]
}
