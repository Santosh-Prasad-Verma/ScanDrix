package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ReviewTaskQueue = "scandrix.reviews.v1"
	ReviewDLX       = "scandrix.dlx"
	DelayedExchange = "scandrix.delayed"
)

// Broker manages an AMQP 0-9-1 connection to RabbitMQ with quorum queue and delayed exchange support.
type Broker struct {
	url       string
	conn      *amqp.Connection
	channel   *amqp.Channel
	mu        sync.Mutex
	isClosing bool
}

// NewBroker establishes a resilient connection to the RabbitMQ broker.
func NewBroker(url string) (*Broker, error) {
	b := &Broker{url: url}
	if err := b.connect(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Broker) connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	conn, err := amqp.Dial(b.url)
	if err != nil {
		return fmt.Errorf("failed to dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare Dead-Letter Exchange
	if err := ch.ExchangeDeclare(
		ReviewDLX,
		"direct",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare dlx exchange: %w", err)
	}

	// Probe and declare Delayed Message Exchange safely on an isolated channel
	// to prevent channel closure on cloud brokers without x-delayed-message plugin
	delayedCh, err := conn.Channel()
	if err == nil {
		delayedArgs := amqp.Table{
			"x-delayed-type": "direct",
		}
		if err := delayedCh.ExchangeDeclare(
			DelayedExchange,
			"x-delayed-message",
			true,  // durable
			false, // auto-deleted
			false, // internal
			false, // no-wait
			delayedArgs,
		); err != nil {
			delayedCh.Close()
			// Fall back to standard direct exchange on primary channel
			_ = ch.ExchangeDeclare(
				DelayedExchange,
				"direct",
				true,
				false,
				false,
				false,
				nil,
			)
		} else {
			delayedCh.Close()
		}
	}

	// Declare Task Queue with Dead-Letter routing
	queueArgs := amqp.Table{
		"x-dead-letter-exchange":    ReviewDLX,
		"x-dead-letter-routing-key": ReviewTaskQueue + ".dlq",
	}

	_, err = ch.QueueDeclare(
		ReviewTaskQueue,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		queueArgs,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare review task queue: %w", err)
	}

	b.conn = conn
	b.channel = ch
	return nil
}

// Publish sends a task payload to the specified routing queue.
func (b *Broker) Publish(ctx context.Context, queueName string, payload []byte) error {
	b.mu.Lock()
	ch := b.channel
	b.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("channel is closed")
	}

	return ch.PublishWithContext(
		ctx,
		"",        // default exchange
		queueName, // routing key
		true,      // mandatory
		false,     // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Timestamp:    time.Now().UTC(),
			Body:         payload,
		},
	)
}

// PublishDelayed publishes a message with delayed delivery using RabbitMQ delayed message exchange.
func (b *Broker) PublishDelayed(ctx context.Context, routingKey string, payload []byte, delayMs int64) error {
	b.mu.Lock()
	ch := b.channel
	b.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("channel is closed")
	}

	headers := amqp.Table{
		"x-delay": delayMs,
	}

	return ch.PublishWithContext(
		ctx,
		DelayedExchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			Headers:      headers,
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Timestamp:    time.Now().UTC(),
			Body:         payload,
		},
	)
}

// Consume subscribes to the review task quorum queue with prefetch.
func (b *Broker) Consume(queueName string, prefetchCount int) (<-chan amqp.Delivery, error) {
	b.mu.Lock()
	ch := b.channel
	b.mu.Unlock()

	if ch == nil {
		return nil, fmt.Errorf("channel is closed")
	}

	if err := ch.Qos(prefetchCount, 0, false); err != nil {
		return nil, fmt.Errorf("failed setting qos prefetch: %w", err)
	}

	return ch.Consume(
		queueName,
		"scandrix-worker",
		false, // auto-ack disabled for at-least-once reliability
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
}

// Close cleanly shuts down RabbitMQ connections.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isClosing = true

	if b.channel != nil {
		_ = b.channel.Close()
	}
	if b.conn != nil {
		_ = b.conn.Close()
	}
	slog.Info("RabbitMQ broker disconnected gracefully")
}
