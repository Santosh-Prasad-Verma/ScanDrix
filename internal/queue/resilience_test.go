package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/queue"
)

func TestResilientConsumerLifecycle(t *testing.T) {
	consumer := queue.NewResilientConsumer(nil, "test-queue", 2, func(ctx context.Context, body []byte) error {
		return nil
	})

	if consumer == nil {
		t.Fatal("expected non-nil resilient consumer")
	}

	ctx, cancel := context.WithCancel(context.Background())
	consumer.Start(ctx)

	// Allow goroutine to start
	time.Sleep(50 * time.Millisecond)

	// Stop consumer
	cancel()
	consumer.Stop()
}
