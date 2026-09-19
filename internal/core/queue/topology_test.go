package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRabbitMQTopology(t *testing.T) {
	top := GetDetailedTopologyMetadata()
	if len(top.Exchanges) == 0 {
		t.Fatalf("expected default exchanges")
	}
	if len(top.Queues) == 0 {
		t.Fatalf("expected default queues")
	}

	foundDelayed := false
	for _, ex := range top.Exchanges {
		if ex.Type == "x-delayed-message" {
			foundDelayed = true
			break
		}
	}
	if !foundDelayed {
		t.Fatalf("expected x-delayed-message exchange in topology")
	}
}

func TestMessageBrokerPublishSubscribe(t *testing.T) {
	broker := NewMessageBrokerService()

	received := make(chan string, 1)
	broker.Subscribe("review.started", func(ctx context.Context, msg MessagePayload) error {
		if str, ok := msg.Data.(string); ok {
			received <- str
		}
		return nil
	})

	err := broker.Publish(context.Background(), "scandrix.workflow.exchange", "review.started", "job-12345")
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	select {
	case res := <-received:
		if res != "job-12345" {
			t.Fatalf("expected job-12345, got %s", res)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for message consumption")
	}
}

func TestMessageBrokerDLQAndDelayed(t *testing.T) {
	broker := NewMessageBrokerService()

	err := broker.PublishDelayed(context.Background(), "review.retry", "payload", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected delayed publish error: %v", err)
	}

	err = broker.PublishToDLQ(context.Background(), "scandrix.workflow.exchange", "review.failed", "bad-payload", "exceeded max retries")
	if err != nil {
		t.Fatalf("unexpected DLQ publish error: %v", err)
	}
}

func TestMessageBrokerConcurrency(t *testing.T) {
	broker := NewMessageBrokerService()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = broker.Publish(context.Background(), "scandrix.workflow.exchange", "concurrent.event", id)
		}(i)
	}
	wg.Wait()
}
