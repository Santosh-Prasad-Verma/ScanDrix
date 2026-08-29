package relay_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/queue/relay"
)

type mockPublisher struct {
	mu        sync.Mutex
	published []string
	shouldErr bool
}

func (m *mockPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr {
		return errors.New("simulated broker connection failure")
	}
	m.published = append(m.published, topic)
	return nil
}

func TestOutboxRelayAndInboxDeduplication(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	store := relay.NewOutboxStore()
	mockPub := &mockPublisher{}
	cfg := relay.DefaultRelayConfig("worker-01")
	cfg.BatchSize = 10

	dispatcher := relay.NewRelayDispatcher(store, mockPub, cfg)

	// 1. Insert 3 messages
	for i := 0; i < 3; i++ {
		err := store.Insert(ctx, relay.OutboxMessage{
			WorkspaceID: wsID,
			Topic:       "scandrix.reviews.events",
			Payload:     []byte(`{"action":"review.started"}`),
			MaxRetries:  2,
		})
		if err != nil {
			t.Fatalf("failed to insert outbox message: %v", err)
		}
	}

	lag, err := store.GetLag(ctx)
	if err != nil || lag.PendingCount != 3 {
		t.Fatalf("expected 3 pending messages, got %+v", lag)
	}

	// 2. Dispatch batch successfully
	dispatched, err := dispatcher.ProcessBatch(ctx)
	if err != nil || dispatched != 3 {
		t.Fatalf("expected 3 dispatched, got %d, err: %v", dispatched, err)
	}

	lagAfter, _ := store.GetLag(ctx)
	if lagAfter.PendingCount != 0 || lagAfter.ClaimedCount != 0 {
		t.Fatalf("expected 0 pending/claimed, got %+v", lagAfter)
	}

	// 3. Test Failure & Dead-Letter Transition
	mockPub.shouldErr = true
	failMsgID := uuid.New()
	_ = store.Insert(ctx, relay.OutboxMessage{
		ID:          failMsgID,
		WorkspaceID: wsID,
		Topic:       "scandrix.reviews.dlq",
		Payload:     []byte(`{"data":"bad"}`),
		MaxRetries:  2,
	})

	// Pass 1: fails, retry_count = 1 (StateFailed)
	_, _ = dispatcher.ProcessBatch(ctx)
	// Pass 2: fails, retry_count = 2 (StateDeadLetter)
	_, _ = dispatcher.ProcessBatch(ctx)

	lagDLQ, _ := store.GetLag(ctx)
	if lagDLQ.DeadLetterCount != 1 {
		t.Fatalf("expected 1 dead-letter message, got %+v", lagDLQ)
	}

	// 4. Test DLQ Redrive
	redriven, err := store.RedriveDeadLetter(ctx, "scandrix.reviews.dlq", 10)
	if err != nil || redriven != 1 {
		t.Fatalf("expected 1 redriven message, got %d", redriven)
	}

	lagRedrive, _ := store.GetLag(ctx)
	if lagRedrive.PendingCount != 1 || lagRedrive.DeadLetterCount != 0 {
		t.Fatalf("expected 1 pending after redrive, got %+v", lagRedrive)
	}

	// 5. Test Inbox Deduplication
	inbox := relay.NewInboxDeduplicator()
	msgKey := "webhook_event_unique_789"
	consumer := "review_worker"

	// First claim must succeed
	claimed, err := inbox.ClaimMessage(ctx, msgKey, consumer)
	if err != nil || !claimed {
		t.Fatalf("expected first claim to succeed, got %v", claimed)
	}

	// Second claim must be rejected as duplicate
	claimed2, err := inbox.ClaimMessage(ctx, msgKey, consumer)
	if err != nil || claimed2 {
		t.Fatalf("expected duplicate claim to be rejected, got %v", claimed2)
	}

	// Mark completed
	if err := inbox.MarkCompleted(ctx, msgKey, consumer); err != nil {
		t.Fatalf("failed to mark inbox completed: %v", err)
	}

	// Third claim after completed must also be rejected
	claimed3, _ := inbox.ClaimMessage(ctx, msgKey, consumer)
	if claimed3 {
		t.Fatalf("expected completed message to reject claim, got true")
	}
}
