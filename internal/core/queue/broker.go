package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/core/log"
)

// MessagePayload represents an AMQP message envelope with metadata.
type MessagePayload struct {
	ID            string                 `json:"id"`
	CorrelationID string                 `json:"correlationId"`
	Type          string                 `json:"type"`
	Timestamp     time.Time              `json:"timestamp"`
	Data          interface{}            `json:"data"`
	Headers       map[string]interface{} `json:"headers,omitempty"`
}

// MessageHandler is the signature for message consumption callbacks.
type MessageHandler func(ctx context.Context, msg MessagePayload) error

// MessageBrokerService manages publishing, subscription, and DLQ dispatch.
type MessageBrokerService struct {
	mu           sync.RWMutex
	topology     *TopologyMetadata
	subscribers  map[string][]MessageHandler
	logger       *log.StructuredLogger
	connected    bool
	inFlightMsgs int64
}

// NewMessageBrokerService creates a new message broker service instance.
func NewMessageBrokerService(topology ...*TopologyMetadata) *MessageBrokerService {
	top := GetDetailedTopologyMetadata()
	if len(topology) > 0 && topology[0] != nil {
		top = topology[0]
	}
	return &MessageBrokerService{
		topology:    top,
		subscribers: make(map[string][]MessageHandler),
		logger:      log.CreateLogger("MessageBrokerService"),
		connected:   true,
	}
}

// Publish dispatches a message to an exchange with a specific routing key.
func (b *MessageBrokerService) Publish(ctx context.Context, exchange, routingKey string, payload interface{}, delayMs ...int) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.connected {
		return fmt.Errorf("message broker disconnected")
	}

	corrID := "corr-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if cid, ok := ctx.Value("correlation_id").(string); ok && cid != "" {
		corrID = cid
	}

	env := MessagePayload{
		ID:            fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		CorrelationID: corrID,
		Type:          routingKey,
		Timestamp:     time.Now().UTC(),
		Data:          payload,
		Headers:       make(map[string]interface{}),
	}

	if len(delayMs) > 0 && delayMs[0] > 0 {
		env.Headers["x-delay"] = delayMs[0]
	}

	b.logger.Debug(log.LogArguments{
		Message:     fmt.Sprintf("Published message to %s:%s", exchange, routingKey),
		Context:     "MessageBroker",
		CorrelationID: corrID,
		Metadata: map[string]interface{}{
			"exchange":   exchange,
			"routingKey": routingKey,
			"msgId":      env.ID,
		},
	})

	// Dispatch to in-process subscribers if matching
	if handlers, ok := b.subscribers[routingKey]; ok {
		for _, h := range handlers {
			go func(handler MessageHandler, msg MessagePayload) {
				_ = handler(ctx, msg)
			}(h, env)
		}
	}

	return nil
}

// Subscribe registers a message consumer callback for a routing key.
func (b *MessageBrokerService) Subscribe(routingKey string, handler MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[routingKey] = append(b.subscribers[routingKey], handler)
	b.logger.Info(log.LogArguments{
		Message: fmt.Sprintf("Subscribed consumer to routing key: %s", routingKey),
		Context: "MessageBroker",
	})
}

// PublishDelayed publishes a delayed message utilizing the delayed message exchange.
func (b *MessageBrokerService) PublishDelayed(ctx context.Context, routingKey string, payload interface{}, delay time.Duration) error {
	delayMs := int(delay.Milliseconds())
	return b.Publish(ctx, "scandrix.orchestrator.exchange.delayed", routingKey, payload, delayMs)
}

// PublishToDLQ publishes a failed message to the Dead Letter Exchange.
func (b *MessageBrokerService) PublishToDLQ(ctx context.Context, originalExchange, routingKey string, payload interface{}, reason string) error {
	raw, _ := json.Marshal(payload)
	b.logger.Warn(log.LogArguments{
		Message: fmt.Sprintf("Routing message to DLQ: %s (reason: %s)", routingKey, reason),
		Context: "MessageBroker",
		Metadata: map[string]interface{}{
			"originalExchange": originalExchange,
			"routingKey":       routingKey,
			"reason":           reason,
			"rawPayload":       string(raw),
		},
	})
	return b.Publish(ctx, "scandrix.orchestrator.exchange.dlx", routingKey+".dead", payload)
}
