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

// ============================================================================
// Thread-Safe Matrix Publisher Double
// ============================================================================

type matrixMockPublisher struct {
	mu           sync.RWMutex
	publishes    []matrixPublishedItem
	publishCount atomic.Int64
	failErr      error
}

type matrixPublishedItem struct {
	Exchange   string
	RoutingKey string
	Mandatory  bool
	Immediate  bool
	Msg        amqp.Publishing
}

func newMatrixMockPublisher() *matrixMockPublisher {
	return &matrixMockPublisher{
		publishes: make([]matrixPublishedItem, 0),
	}
}

func (p *matrixMockPublisher) PublishWithContext(
	ctx context.Context,
	exchange, key string,
	mandatory, immediate bool,
	msg amqp.Publishing,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failErr != nil {
		return p.failErr
	}
	p.publishCount.Add(1)
	p.publishes = append(p.publishes, matrixPublishedItem{
		Exchange:   exchange,
		RoutingKey: key,
		Mandatory:  mandatory,
		Immediate:  immediate,
		Msg:        msg,
	})
	return nil
}

func (p *matrixMockPublisher) GetPublishes() []matrixPublishedItem {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cp := make([]matrixPublishedItem, len(p.publishes))
	copy(cp, p.publishes)
	return cp
}

// ============================================================================
// Matrix Tests: Centralized Topology Invariants
// ============================================================================

func TestRabbitMQMatrix_TopologyConfigurationInvariants(t *testing.T) {
	configs := queue.RabbitMQTopologyConfig
	require.NotEmpty(t, configs)

	for _, ex := range configs {
		t.Run("Exchange_"+ex.Name, func(t *testing.T) {
			assert.NotEmpty(t, ex.Name)
			assert.True(t, ex.Durable, "All enterprise ScanDrix exchanges must be durable")

			if ex.Type == "x-delayed-message" {
				require.NotNil(t, ex.Options)
				assert.Contains(t, ex.Options, "x-delayed-type")
				assert.Contains(t, []string{"direct", "topic"}, ex.Options["x-delayed-type"])
			} else {
				assert.Equal(t, "topic", ex.Type)
			}
		})
	}
}

// ============================================================================
// Matrix Tests: Delay Calculation & RateLimit Backoff
// ============================================================================

func TestRabbitMQMatrix_DelayCalculationMatrix(t *testing.T) {
	pub := newMatrixMockPublisher()
	handler := queue.NewRabbitMQErrorHandler(pub, 3, 1000)

	t.Run("RateLimitError with Future Reset and Safety Buffer", func(t *testing.T) {
		resetTime := time.Now().Add(30 * time.Second)
		rateErr := &queue.RateLimitError{
			Message: "GitHub secondary rate limit reached",
			ResetAt: resetTime,
		}

		delivery := amqp.Delivery{
			Exchange:   "workflow.exchange",
			RoutingKey: "review.analyze",
			Body:       []byte(`{"pr": 101}`),
		}

		err := handler.Handle(context.Background(), delivery, rateErr, "")
		require.NoError(t, err)

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 1)

		delayVal, ok := publishes[0].Msg.Headers[queue.HeaderDelay].(int64)
		require.True(t, ok)

		// 30s + 5m safety buffer = 330,000ms (~330s). Tolerating execution latency
		assert.True(t, delayVal >= 329000 && delayVal <= 331000, "delay was %d ms", delayVal)
	})

	t.Run("RateLimitError Capped at 1 Hour Max Delay", func(t *testing.T) {
		pub.mu.Lock()
		pub.publishes = nil
		pub.mu.Unlock()

		resetTime := time.Now().Add(2 * time.Hour) // Far future
		rateErr := &queue.RateLimitError{
			Message: "Provider quota reset in 2 hours",
			ResetAt: resetTime,
		}

		delivery := amqp.Delivery{
			Exchange:   "workflow.exchange",
			RoutingKey: "review.analyze",
			Body:       []byte(`{"pr": 102}`),
		}

		err := handler.Handle(context.Background(), delivery, rateErr, "")
		require.NoError(t, err)

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 1)

		delayVal, ok := publishes[0].Msg.Headers[queue.HeaderDelay].(int64)
		require.True(t, ok)
		assert.Equal(t, int64(queue.RateLimitedMaxDelayMs), delayVal) // exactly 1h
	})

	t.Run("RateLimitError with Past Reset Uses Full Safety Buffer", func(t *testing.T) {
		pub.mu.Lock()
		pub.publishes = nil
		pub.mu.Unlock()

		resetTime := time.Now().Add(-10 * time.Second) // In the past
		rateErr := &queue.RateLimitError{
			Message: "Expired rate limit bucket",
			ResetAt: resetTime,
		}

		delivery := amqp.Delivery{
			Exchange:   "workflow.exchange",
			RoutingKey: "review.analyze",
		}

		err := handler.Handle(context.Background(), delivery, rateErr, "")
		require.NoError(t, err)

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 1)

		delayVal, ok := publishes[0].Msg.Headers[queue.HeaderDelay].(int64)
		require.True(t, ok)
		assert.Equal(t, int64(queue.RateLimitedSafetyBufferMs), delayVal) // 5 minutes
	})

	t.Run("Exponential Backoff Generic Progression", func(t *testing.T) {
		pub.mu.Lock()
		pub.publishes = nil
		pub.mu.Unlock()

		standardErr := errors.New("temporary connection reset by peer")

		// Attempt 1 (retryCount 0 -> 1): 1000 * 2^0 = 1000ms
		d1 := amqp.Delivery{
			Exchange:   "workflow.exchange",
			RoutingKey: "step1",
		}
		require.NoError(t, handler.Handle(context.Background(), d1, standardErr, ""))

		// Attempt 2 (retryCount 1 -> 2): 1000 * 2^1 = 2000ms
		d2 := amqp.Delivery{
			Exchange:   "workflow.exchange",
			RoutingKey: "step2",
			Headers:    amqp.Table{queue.HeaderRetryCount: int32(1)},
		}
		require.NoError(t, handler.Handle(context.Background(), d2, standardErr, ""))

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 2)

		assert.Equal(t, int64(1000), publishes[0].Msg.Headers[queue.HeaderDelay])
		assert.Equal(t, int64(2000), publishes[1].Msg.Headers[queue.HeaderDelay])
	})
}

// ============================================================================
// Matrix Tests: Delivery Header Preservation & Dead Letter Queue Routing
// ============================================================================

func TestRabbitMQMatrix_HeaderPreservationAndDLQ(t *testing.T) {
	pub := newMatrixMockPublisher()
	maxRetries := 2
	handler := queue.NewRabbitMQErrorHandler(pub, maxRetries, 500)

	t.Run("Header Preservation Across Delayed Retry", func(t *testing.T) {
		msgTimestamp := time.Now().UTC().Truncate(time.Second)
		delivery := amqp.Delivery{
			Exchange:        "workflow.events",
			RoutingKey:      "pr.opened",
			ContentType:     "application/json",
			ContentEncoding: "gzip",
			DeliveryMode:    amqp.Persistent,
			Priority:        9,
			CorrelationId:   "corr-999-matrix",
			MessageId:       "msg-abc-001",
			Timestamp:       msgTimestamp,
			Type:            "CodeReviewTriggeredEvent",
			UserId:          "svc-account",
			AppId:           "scandrix-webhooks",
			Body:            []byte(`{"pr_id": 999}`),
			Headers: amqp.Table{
				"x-custom-tenant": "tenant-matrix-777",
			},
		}

		err := handler.Handle(context.Background(), delivery, errors.New("database locked"), "")
		require.NoError(t, err)

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 1)
		p := publishes[0]

		assert.Equal(t, "workflow.events.delayed", p.Exchange)
		assert.Equal(t, "pr.opened", p.RoutingKey)

		// Check message attributes preserved
		assert.Equal(t, "application/json", p.Msg.ContentType)
		assert.Equal(t, "gzip", p.Msg.ContentEncoding)
		assert.Equal(t, amqp.Persistent, p.Msg.DeliveryMode)
		assert.Equal(t, uint8(9), p.Msg.Priority)
		assert.Equal(t, "corr-999-matrix", p.Msg.CorrelationId)
		assert.Equal(t, "msg-abc-001", p.Msg.MessageId)
		assert.Equal(t, msgTimestamp, p.Msg.Timestamp)
		assert.Equal(t, "CodeReviewTriggeredEvent", p.Msg.Type)
		assert.Equal(t, "svc-account", p.Msg.UserId)
		assert.Equal(t, "scandrix-webhooks", p.Msg.AppId)
		assert.Equal(t, []byte(`{"pr_id": 999}`), p.Msg.Body)

		// Check retry headers enriched
		assert.Equal(t, int32(1), p.Msg.Headers[queue.HeaderRetryCount])
		assert.Equal(t, "workflow.events", p.Msg.Headers[queue.HeaderOriginalExchange])
		assert.Equal(t, "pr.opened", p.Msg.Headers[queue.HeaderOriginalRoutingKey])
		assert.Equal(t, "tenant-matrix-777", p.Msg.Headers["x-custom-tenant"])
	})

	t.Run("Max Retries Exceeded Routes to DLX and Records Error", func(t *testing.T) {
		pub.mu.Lock()
		pub.publishes = nil
		pub.mu.Unlock()

		fatalErr := errors.New("syntax error: unrecoverable AST parsing failure")
		delivery := amqp.Delivery{
			Exchange:   "workflow.exchange",
			RoutingKey: "review.ast.build",
			Body:       []byte(`{"commit": "sha-broken"}`),
			Headers: amqp.Table{
				queue.HeaderRetryCount: int32(2), // Already at maxRetries (2)
			},
		}

		err := handler.Handle(context.Background(), delivery, fatalErr, "dlq.custom.failed")
		require.NoError(t, err)

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 1)
		p := publishes[0]

		assert.Equal(t, "workflow.exchange.dlx", p.Exchange)
		assert.Equal(t, "dlq.custom.failed", p.RoutingKey)
		assert.Equal(t, amqp.Persistent, p.Msg.DeliveryMode)
		assert.Equal(t, fatalErr.Error(), p.Msg.Headers[queue.HeaderOriginalError])
	})

	t.Run("Default DLQ Routing Key Fallback", func(t *testing.T) {
		pub.mu.Lock()
		pub.publishes = nil
		pub.mu.Unlock()

		delivery := amqp.Delivery{
			Exchange:   "notification.exchange",
			RoutingKey: "slack.alert",
			Headers: amqp.Table{
				queue.HeaderRetryCount: int32(2),
			},
		}

		// Passing empty dlqRoutingKey uses default "workflow.job.failed"
		err := handler.Handle(context.Background(), delivery, errors.New("slack webhook 404"), "")
		require.NoError(t, err)

		publishes := pub.GetPublishes()
		require.Len(t, publishes, 1)
		p := publishes[0]

		assert.Equal(t, "notification.exchange.dlx", p.Exchange)
		assert.Equal(t, "workflow.job.failed", p.RoutingKey)
	})
}

// ============================================================================
// Matrix Tests: High Concurrency Multithreaded Failure Routing Stress
// ============================================================================

func TestRabbitMQMatrix_HighConcurrencyFailureRoutingStress(t *testing.T) {
	pub := newMatrixMockPublisher()
	maxRetries := 2
	handler := queue.NewRabbitMQErrorHandler(pub, maxRetries, 100)

	const numWorkers = 50
	const messagesPerWorker = 20

	var totalProcessed atomic.Int64
	var totalRetried atomic.Int64
	var totalDLQ atomic.Int64

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	ctx := context.Background()

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < messagesPerWorker; i++ {
				// Alternate between initial failure (retryCount = 0) and final failure (retryCount = 2)
				isFinal := (workerID*messagesPerWorker+i)%2 == 1

				var headers amqp.Table
				if isFinal {
					headers = amqp.Table{queue.HeaderRetryCount: int32(2)}
				}

				delivery := amqp.Delivery{
					Exchange:   "workflow.exchange",
					RoutingKey: fmt.Sprintf("job.worker.%d", workerID),
					Body:       []byte(fmt.Sprintf(`{"worker": %d, "msg": %d}`, workerID, i)),
					Headers:    headers,
				}

				err := handler.Handle(ctx, delivery, errors.New("worker simulated error"), "dlq.stress")
				if err == nil {
					totalProcessed.Add(1)
					if isFinal {
						totalDLQ.Add(1)
					} else {
						totalRetried.Add(1)
					}
				}
			}
		}(w)
	}

	wg.Wait()

	expectedTotal := int64(numWorkers * messagesPerWorker)
	assert.Equal(t, expectedTotal, totalProcessed.Load())
	assert.Equal(t, expectedTotal/2, totalRetried.Load())
	assert.Equal(t, expectedTotal/2, totalDLQ.Load())
	assert.Equal(t, expectedTotal, pub.publishCount.Load())
}
