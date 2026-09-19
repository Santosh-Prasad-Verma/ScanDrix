// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package lifecycle

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_PubSubAndHistory(t *testing.T) {
	bus := NewEventBus(10)

	var turnStartedCount int32
	var lastTurnID string

	bus.Subscribe(EventTurnStarted, func(e LifecycleEvent) {
		atomic.AddInt32(&turnStartedCount, 1)
		lastTurnID = e.TurnID
	})

	bus.Emit(LifecycleEvent{
		Type:      EventTurnStarted,
		TurnID:    "turn-101",
		SessionID: "sess-abc",
		Payload:   map[string]any{"branch": "feat/login"},
	})

	bus.Emit(LifecycleEvent{
		Type:      EventTurnCompleted,
		TurnID:    "turn-101",
		SessionID: "sess-abc",
		Payload:   map[string]any{"duration_ms": 150},
	})

	if atomic.LoadInt32(&turnStartedCount) != 1 {
		t.Errorf("expected 1 turn started event, got %d", turnStartedCount)
	}
	if lastTurnID != "turn-101" {
		t.Errorf("expected turn-101, got %s", lastTurnID)
	}

	history := bus.History()
	if len(history) != 2 {
		t.Errorf("expected 2 history items, got %d", len(history))
	}

	turnEvents := bus.FilterHistory(EventTurnStarted)
	if len(turnEvents) != 1 || turnEvents[0].TurnID != "turn-101" {
		t.Errorf("unexpected filtered history: %+v", turnEvents)
	}

	bus.ClearHistory()
	if len(bus.History()) != 0 {
		t.Errorf("expected empty history after clear")
	}
}

func TestEventBus_BoundedHistory(t *testing.T) {
	bus := NewEventBus(3)

	for i := 1; i <= 5; i++ {
		bus.Emit(LifecycleEvent{
			Type:      EventHookInvoked,
			Timestamp: time.Now(),
			Payload:   map[string]any{"index": i},
		})
	}

	history := bus.History()
	if len(history) != 3 {
		t.Fatalf("expected bounded history of 3, got %d", len(history))
	}
	if history[2].Payload["index"] != 5 {
		t.Errorf("expected newest event to be index 5, got %+v", history[2].Payload)
	}
}
