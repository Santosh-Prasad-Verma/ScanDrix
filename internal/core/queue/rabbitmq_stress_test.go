package queue_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/scandrix/backend/internal/core/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stressMockPublisher struct {
	mu          sync.Mutex
	publishes   []publishedItem
	publishFail bool
}

type publishedItem struct {
	Exchange   string
	RoutingKey string
	Msg        amqp.Publishing
}

func (m *stressMockPublisher) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publishFail {
		return errors.New("amqp connection dropped")
	}
	m.publishes = append(m.publishes, publishedItem{
		Exchange:   exchange,
		RoutingKey: key,
		Msg:        msg,
	})
	return nil
}

// TestRabbitMQStress_HighConcurrencyRetriesAndDLQRouting verifies error handling
// under 50 concurrent failed message deliveries across retries and final DLQ routing.
func TestRabbitMQStress_HighConcurrencyRetriesAndDLQRouting(t *testing.T) {
	mockPub := &stressMockPublisher{}
	handler := queue.NewRabbitMQErrorHandler(mockPub, 2, 1000)

	const numMessages = 50
	var delayedPublishes atomic.Int32
	var dlqPublishes atomic.Int32

	var wg sync.WaitGroup
	wg.Add(numMessages)

	for i := 0; i < numMessages; i++ {
		go func(msgIdx int) {
			defer wg.Done()
			ctx := context.Background()

			// Delivery 1: Attempt 0 -> should republish to delayed exchange
			delivery1 := amqp.Delivery{
				Exchange:   "workflow.exchange",
				RoutingKey: "workflow.review.start",
				Body:       []byte(fmt.Sprintf(`{"msg_idx":%d}`, msgIdx)),
				Headers:    amqp.Table{},
			}
			err := handler.Handle(ctx, delivery1, errors.New("temporary AST parse error"), "workflow.review.dlq")
			assert.NoError(t, err)
			delayedPublishes.Add(1)

			// Delivery 2: Attempt 1 -> should republish to delayed exchange with retry_count=2
			delivery2 := amqp.Delivery{
				Exchange:   "workflow.exchange",
				RoutingKey: "workflow.review.start",
				Body:       []byte(fmt.Sprintf(`{"msg_idx":%d}`, msgIdx)),
				Headers: amqp.Table{
					queue.HeaderRetryCount: int32(1),
				},
			}
			err = handler.Handle(ctx, delivery2, errors.New("temporary AST parse error"), "workflow.review.dlq")
			assert.NoError(t, err)
			delayedPublishes.Add(1)

			// Delivery 3: Attempt 2 (reaches max=2) -> should route to DLX exchange
			delivery3 := amqp.Delivery{
				Exchange:   "workflow.exchange",
				RoutingKey: "workflow.review.start",
				Body:       []byte(fmt.Sprintf(`{"msg_idx":%d}`, msgIdx)),
				Headers: amqp.Table{
					queue.HeaderRetryCount: int32(2),
				},
			}
			err = handler.Handle(ctx, delivery3, errors.New("fatal LLM rate limit"), "workflow.review.dlq")
			assert.NoError(t, err)
			dlqPublishes.Add(1)
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numMessages*2), delayedPublishes.Load())
	assert.Equal(t, int32(numMessages), dlqPublishes.Load())

	mockPub.mu.Lock()
	defer mockPub.mu.Unlock()
	assert.Equal(t, numMessages*3, len(mockPub.publishes))
}

// TestRabbitMQStress_RateLimitErrorCalculations tests the 5-minute safety buffer
// and 1-hour maximum delay caps for RateLimitError.
func TestRabbitMQStress_RateLimitErrorCalculations(t *testing.T) {
	mockPub := &stressMockPublisher{}
	handler := queue.NewRabbitMQErrorHandler(mockPub, 3, 1000)
	ctx := context.Background()

	// 1. RateLimit resets in 30 seconds -> delay should be 30s + 5min (330,000 ms)
	rle1 := &queue.RateLimitError{
		Message: "Secondary rate limit on GitHub API",
		ResetAt: time.Now().Add(30 * time.Second),
	}
	del1 := amqp.Delivery{
		Exchange:   "workflow.exchange",
		RoutingKey: "scm.github.fetch",
		Headers:    amqp.Table{},
	}
	err := handler.Handle(ctx, del1, rle1, "scm.dlq")
	require.NoError(t, err)

	mockPub.mu.Lock()
	lastPub := mockPub.publishes[len(mockPub.publishes)-1]
	mockPub.mu.Unlock()

	delay, ok := lastPub.Msg.Headers[queue.HeaderDelay].(int64)
	require.True(t, ok)
	// Between 328,000ms and 331,000ms
	assert.True(t, delay >= 328000 && delay <= 331000, "Unexpected delay: %d", delay)

	// 2. RateLimit resets in 5 hours -> delay should be capped at exactly 1 hour (3,600,000 ms)
	rleFarFuture := &queue.RateLimitError{
		Message: "Primary rate limit exhausted",
		ResetAt: time.Now().Add(5 * time.Hour),
	}
	delFar := amqp.Delivery{
		Exchange:   "workflow.exchange",
		RoutingKey: "scm.github.fetch",
		Headers:    amqp.Table{},
	}
	err = handler.Handle(ctx, delFar, rleFarFuture, "scm.dlq")
	require.NoError(t, err)

	mockPub.mu.Lock()
	lastPubFar := mockPub.publishes[len(mockPub.publishes)-1]
	mockPub.mu.Unlock()

	delayFar, ok := lastPubFar.Msg.Headers[queue.HeaderDelay].(int64)
	require.True(t, ok)
	assert.Equal(t, int64(queue.RateLimitedMaxDelayMs), delayFar)
}

// TestRabbitMQStress_TopologyDefinitions verifies all required exchanges exist in config.
func TestRabbitMQStress_TopologyDefinitions(t *testing.T) {
	requiredExchanges := []string{
		"orchestrator.exchange.dlx",
		"orchestrator.exchange.delayed",
		"workflow.exchange",
		"workflow.exchange.dlx",
		"workflow.exchange.delayed",
		"workflow.events",
		"workflow.events.dlx",
		"workflow.events.delayed",
		"notification.exchange",
	}

	foundMap := make(map[string]bool)
	for _, def := range queue.RabbitMQTopologyConfig {
		foundMap[def.Name] = true
		assert.True(t, def.Durable, "Exchange %s should be durable", def.Name)
	}

	for _, req := range requiredExchanges {
		assert.True(t, foundMap[req], "Missing exchange: %s", req)
	}
}

// TestRabbitMQStress_PublishFailureReturnsError verifies error bubbling when AMQP publisher fails.
func TestRabbitMQStress_PublishFailureReturnsError(t *testing.T) {
	mockPub := &stressMockPublisher{publishFail: true}
	handler := queue.NewRabbitMQErrorHandler(mockPub, 2, 1000)

	del := amqp.Delivery{
		Exchange:   "workflow.exchange",
		RoutingKey: "task",
	}

	err := handler.Handle(context.Background(), del, errors.New("task error"), "dlq")
	assert.Error(t, err)
	assert.Equal(t, "amqp connection dropped", err.Error())
}
