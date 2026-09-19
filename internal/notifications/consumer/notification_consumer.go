package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/scandrix/backend/internal/notifications/application"
)

// NotificationConsumer consumes notification messages from RabbitMQ and runs dispatcher fanout.
type NotificationConsumer struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	dispatcher *application.NotificationDispatcherService
	queueName  string
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewNotificationConsumer creates a RabbitMQ notification consumer.
func NewNotificationConsumer(
	conn *amqp.Connection,
	dispatcher *application.NotificationDispatcherService,
	queueName string,
) *NotificationConsumer {
	if queueName == "" {
		queueName = "notifications.worker.queue"
	}
	return &NotificationConsumer{
		conn:       conn,
		dispatcher: dispatcher,
		queueName:  queueName,
		stopCh:     make(chan struct{}),
	}
}

// Start begins listening to the RabbitMQ queue.
func (c *NotificationConsumer) Start(ctx context.Context) error {
	if c.conn == nil {
		return fmt.Errorf("amqp connection is nil")
	}

	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open amqp channel: %w", err)
	}
	c.channel = ch

	// Declare exchange and queue
	exchangeName := "notification.exchange"
	err = ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	q, err := ch.QueueDeclare(
		c.queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		amqp.Table{"x-queue-type": "quorum"},
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	err = ch.QueueBind(
		q.Name,
		"notification.*",
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}

	msgs, err := ch.Consume(
		q.Name,
		"scandrix-notifications-worker",
		false, // manual ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				c.handleDelivery(ctx, msg)
			}
		}
	}()

	return nil
}

func (c *NotificationConsumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	var notifMsg application.NotificationMessage
	if err := json.Unmarshal(delivery.Body, &notifMsg); err != nil {
		log.Printf("[NotificationConsumer] Failed to unmarshal message: %v", err)
		_ = delivery.Reject(false) // Don't requeue malformed JSON
		return
	}

	msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := c.dispatcher.Dispatch(msgCtx, notifMsg)
	if err != nil {
		log.Printf("[NotificationConsumer] Unhandled error dispatching event %s: %v", notifMsg.Event, err)
		// Nack and requeue for transient recovery
		_ = delivery.Nack(false, true)
		return
	}

	_ = delivery.Ack(false)
}

// Stop terminates the consumer gracefully.
func (c *NotificationConsumer) Stop() {
	close(c.stopCh)
	if c.channel != nil {
		_ = c.channel.Close()
	}
	c.wg.Wait()
}
