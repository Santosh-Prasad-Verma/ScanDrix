package streaming_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/streaming"
)

func TestStreamBrokerPubSub(t *testing.T) {
	broker := streaming.NewStreamBroker()
	reviewID := uuid.New()

	// 1. Subscribe 3 clients
	sub1 := broker.Subscribe(reviewID, "client-1")
	sub2 := broker.Subscribe(reviewID, "client-2")
	sub3 := broker.Subscribe(reviewID, "client-3")

	if broker.SubscriberCount(reviewID) != 3 {
		t.Fatalf("expected 3 subscribers, got %d", broker.SubscriberCount(reviewID))
	}

	// 2. Broadcast Event
	event1 := streaming.StreamEvent{
		ReviewID: reviewID,
		Type:     streaming.EventStageTransition,
		Payload: streaming.StageTransitionPayload{
			StageName: "AST_COMPLEXITY_ANALYSIS",
			Progress:  0.45,
			Message:   "Analyzing cyclomatic and cognitive complexity",
		},
	}

	delivered := broker.Broadcast(event1)
	if delivered != 3 {
		t.Fatalf("expected 3 deliveries, got %d", delivered)
	}

	// Verify all 3 received
	select {
	case ev := <-sub1.EventChan:
		if ev.Type != streaming.EventStageTransition {
			t.Fatalf("unexpected event type for sub1: %s", ev.Type)
		}
	default:
		t.Fatal("sub1 did not receive event")
	}

	select {
	case ev := <-sub2.EventChan:
		if ev.Type != streaming.EventStageTransition {
			t.Fatalf("unexpected event type for sub2: %s", ev.Type)
		}
	default:
		t.Fatal("sub2 did not receive event")
	}

	// 3. Unsubscribe client 3
	broker.Unsubscribe(sub3)
	if broker.SubscriberCount(reviewID) != 2 {
		t.Fatalf("expected 2 subscribers after unsubscribe, got %d", broker.SubscriberCount(reviewID))
	}

	broker.Unsubscribe(sub1)
	broker.Unsubscribe(sub2)
	if broker.SubscriberCount(reviewID) != 0 {
		t.Fatalf("expected 0 subscribers after all unsubscribe, got %d", broker.SubscriberCount(reviewID))
	}
}

func TestSSEHandlerStreamingOutput(t *testing.T) {
	broker := streaming.NewStreamBroker()
	handler := streaming.NewSSEHandler(broker)
	server := httptest.NewServer(handler)
	defer server.Close()

	reviewID := uuid.New()
	url := fmt.Sprintf("%s/stream?review_id=%s", server.URL, reviewID.String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed connecting to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream Content-Type, got %s", ct)
	}

	reader := bufio.NewReader(resp.Body)

	// Read initial heartbeat line
	line1, _ := reader.ReadString('\n')
	if !strings.HasPrefix(line1, "id:") {
		t.Fatalf("expected id line, got %s", line1)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Broadcast an actual stage transition event
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond) // Ensure subscription is established
		broker.Broadcast(streaming.StreamEvent{
			ReviewID: reviewID,
			Type:     streaming.EventFindingDiscovered,
			Payload: map[string]string{
				"title":    "SQL Injection in auth handler",
				"severity": "CRITICAL",
			},
		})
	}()

	// Read until we encounter the broadcasted event
	foundFindingEvent := false
	for i := 0; i < 15; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.Contains(line, "FINDING_DISCOVERED") {
			foundFindingEvent = true
			break
		}
	}

	wg.Wait()

	if !foundFindingEvent {
		t.Fatal("expected to receive FINDING_DISCOVERED event over SSE connection")
	}
}
