package queue

import (
	"context"
	"fmt"
	"math"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Constants from ScanDrix rabbitmq-error.handler.ts.
const (
	DefaultMaxRetriesConsumer  = 2
	DefaultRetryDelayMs        = 1000
	RateLimitedSafetyBufferMs  = 5 * 60 * 1000  // 5 minutes
	RateLimitedMaxDelayMs      = 60 * 60 * 1000 // 1 hour
	HeaderRetryCount           = "x-retry-count"
	HeaderOriginalExchange     = "x-original-exchange"
	HeaderOriginalRoutingKey   = "x-original-routing-key"
	HeaderOriginalError        = "x-original-error"
	HeaderDelay                = "x-delay"
)

// ExchangeDefinition defines RabbitMQ topology configuration matching RABBITMQ_TOPOLOGY_CONFIG.
type ExchangeDefinition struct {
	Name    string
	Type    string
	Durable bool
	Options map[string]any
}

// RabbitMQTopologyConfig mirrors ScanDrix centralized topology configuration.
var RabbitMQTopologyConfig = []ExchangeDefinition{
	// Core exchanges
	{Name: "orchestrator.exchange.dlx", Type: "topic", Durable: true},
	{Name: "orchestrator.exchange.delayed", Type: "x-delayed-message", Durable: true, Options: map[string]any{"x-delayed-type": "direct"}},

	// Workflow domain exchanges
	{Name: "workflow.exchange", Type: "topic", Durable: true},
	{Name: "workflow.exchange.dlx", Type: "topic", Durable: true},
	{Name: "workflow.exchange.delayed", Type: "x-delayed-message", Durable: true, Options: map[string]any{"x-delayed-type": "topic"}},
	{Name: "workflow.events", Type: "topic", Durable: true},
	{Name: "workflow.events.dlx", Type: "topic", Durable: true},
	{Name: "workflow.events.delayed", Type: "x-delayed-message", Durable: true, Options: map[string]any{"x-delayed-type": "topic"}},

	// Notifications domain
	{Name: "notification.exchange", Type: "topic", Durable: true},
}

// RateLimitError mirrors ScanDrix RateLimitError carrying the GitHub App bucket reset timestamp.
type RateLimitError struct {
	Message string
	ResetAt time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded: %s (resets at %v)", e.Message, e.ResetAt)
}

// ChannelPublisher abstracts AMQP publishing for unit testing and production.
type ChannelPublisher interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// RabbitMQErrorHandler mirrors ScanDrix RabbitMQErrorHandler.
type RabbitMQErrorHandler struct {
	publisher          ChannelPublisher
	maxRetriesConsumer int
	retryDelayMs       int
}

// NewRabbitMQErrorHandler instantiates an error handler with ScanDrix defaults.
func NewRabbitMQErrorHandler(publisher ChannelPublisher, maxRetries, retryDelayMs int) *RabbitMQErrorHandler {
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetriesConsumer
	}
	if retryDelayMs <= 0 {
		retryDelayMs = DefaultRetryDelayMs
	}
	return &RabbitMQErrorHandler{
		publisher:          publisher,
		maxRetriesConsumer: maxRetries,
		retryDelayMs:       retryDelayMs,
	}
}

// Handle handles message consumption failures, retrying with delayed exchange or routing to DLQ.
func (h *RabbitMQErrorHandler) Handle(
	ctx context.Context,
	delivery amqp.Delivery,
	consumeErr error,
	dlqRoutingKey string,
) error {
	headers := make(amqp.Table)
	for k, v := range delivery.Headers {
		headers[k] = v
	}

	retryCount := 0
	if rc, ok := headers[HeaderRetryCount].(int32); ok {
		retryCount = int(rc)
	} else if rc, ok := headers[HeaderRetryCount].(int); ok {
		retryCount = rc
	}

	baseExchange := delivery.Exchange
	if baseExchange == "" {
		baseExchange = "workflow.exchange"
	}
	delayedExchange := fmt.Sprintf("%s.delayed", baseExchange)
	dlxExchange := fmt.Sprintf("%s.dlx", baseExchange)

	if retryCount < h.maxRetriesConsumer {
		// Retry with delay
		nextCount := retryCount + 1
		headers[HeaderRetryCount] = int32(nextCount)
		headers[HeaderOriginalExchange] = delivery.Exchange
		headers[HeaderOriginalRoutingKey] = delivery.RoutingKey

		delayMs := h.calculateRetryDelay(consumeErr, nextCount)
		headers[HeaderDelay] = delayMs

		pub := amqp.Publishing{
			Headers:         headers,
			ContentType:     delivery.ContentType,
			ContentEncoding: delivery.ContentEncoding,
			DeliveryMode:    delivery.DeliveryMode,
			Priority:        delivery.Priority,
			CorrelationId:   delivery.CorrelationId,
			MessageId:       delivery.MessageId,
			Timestamp:       delivery.Timestamp,
			Type:            delivery.Type,
			UserId:          delivery.UserId,
			AppId:           delivery.AppId,
			Body:            delivery.Body,
		}

		return h.publisher.PublishWithContext(ctx, delayedExchange, delivery.RoutingKey, false, false, pub)
	}

	// Max retries exceeded: publish to DLQ
	if dlqRoutingKey == "" {
		dlqRoutingKey = "workflow.job.failed"
	}
	headers[HeaderOriginalError] = consumeErr.Error()

	pub := amqp.Publishing{
		Headers:      headers,
		DeliveryMode: amqp.Persistent,
		Body:         delivery.Body,
	}

	return h.publisher.PublishWithContext(ctx, dlxExchange, dlqRoutingKey, false, false, pub)
}

func (h *RabbitMQErrorHandler) calculateRetryDelay(err error, attempt int) int64 {
	// If RateLimitError, wait until bucket resets + safety buffer, capped at 1h
	if rle, ok := err.(*RateLimitError); ok {
		remMs := time.Until(rle.ResetAt).Milliseconds()
		if remMs < 0 {
			remMs = 0
		}
		totalDelay := remMs + int64(RateLimitedSafetyBufferMs)
		if totalDelay > int64(RateLimitedMaxDelayMs) {
			totalDelay = int64(RateLimitedMaxDelayMs)
		}
		return totalDelay
	}

	// Generic exponential backoff: retryDelayMs * 2^(attempt-1)
	factor := math.Pow(2, float64(attempt-1))
	delay := int64(float64(h.retryDelayMs) * factor)
	if delay > int64(RateLimitedMaxDelayMs) {
		delay = int64(RateLimitedMaxDelayMs)
	}
	return delay
}
