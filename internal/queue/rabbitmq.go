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

	// Declare Task Queue with Dead-Letter routing and quorum replication
	queueArgs := amqp.Table{
		"x-queue-type":              "quorum",
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
		// If queue already exists on the broker as classic (406 inequivalent arg),
		// reopen the channel and declare with classic fallback to support existing brokers
		_ = ch.Close()
		ch, err = conn.Channel()
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("failed to reopen channel after quorum probe: %w", err)
		}
		fallbackArgs := amqp.Table{
			"x-dead-letter-exchange":    ReviewDLX,
			"x-dead-letter-routing-key": ReviewTaskQueue + ".dlq",
		}
		_, err = ch.QueueDeclare(
			ReviewTaskQueue,
			true,  // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			fallbackArgs,
		)
		if err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return fmt.Errorf("failed to declare review task queue: %w", err)
		}
	}

	b.conn = conn
	b.channel = ch
	return nil
}

// Publish sends a task payload to the specified routing queue safely with thread serialization.
func (b *Broker) Publish(ctx context.Context, queueName string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.channel == nil || b.channel.IsClosed() {
		if b.conn != nil && !b.conn.IsClosed() {
			var err error
			b.channel, err = b.conn.Channel()
			if err != nil {
				return fmt.Errorf("failed to reopen channel: %w", err)
			}
		} else {
			return fmt.Errorf("broker channel and connection are closed")
		}
	}

	return b.channel.PublishWithContext(
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
	defer b.mu.Unlock()

	if b.channel == nil || b.channel.IsClosed() {
		if b.conn != nil && !b.conn.IsClosed() {
			var err error
			b.channel, err = b.conn.Channel()
			if err != nil {
				return fmt.Errorf("failed to reopen channel: %w", err)
			}
		} else {
			return fmt.Errorf("broker channel and connection are closed")
		}
	}

	headers := amqp.Table{
		"x-delay": delayMs,
	}

	return b.channel.PublishWithContext(
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

// Consume subscribes to the review task quorum queue with a dedicated AMQP channel to avoid channel contention.
func (b *Broker) Consume(queueName string, prefetchCount int) (<-chan amqp.Delivery, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn == nil || b.conn.IsClosed() {
		return nil, fmt.Errorf("broker connection is closed")
	}

	// Open a dedicated channel for this consumer so it does not collide with concurrent publishes
	consumerCh, err := b.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed opening dedicated consumer channel: %w", err)
	}

	if err := consumerCh.Qos(prefetchCount, 0, false); err != nil {
		_ = consumerCh.Close()
		return nil, fmt.Errorf("failed setting qos prefetch: %w", err)
	}

	return consumerCh.Consume(
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
