package sse_test

import (
	"testing"
	"time"

	"github.com/codehound/codehound/core/pkg/sse"
)

func TestSSEBrokerPublishAndSubscribe(t *testing.T) {
	broker := sse.NewBroker()
	auditID := "test-audit-123"

	ch, unsubscribe := broker.Subscribe(auditID)
	defer unsubscribe()

	testEvt := sse.StreamEvent{
		Event:   "stage_started",
		AuditID: auditID,
		Stage:   "AST_PARSING",
		Percent: 10,
	}

	broker.Publish(auditID, testEvt)

	select {
	case received := <-ch:
		if received.Event != "stage_started" || received.Stage != "AST_PARSING" {
			t.Errorf("received event mismatch: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for SSE event")
	}
}
