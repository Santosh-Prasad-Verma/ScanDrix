package queue

import (
	"context"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// TaskHandler defines the worker callback function for processing incoming deliveries.
type TaskHandler func(ctx context.Context, body []byte) error

// ResilientConsumer manages a bounded worker pool and automatic reconnection loop.
type ResilientConsumer struct {
	broker      *Broker
	queueName   string
	workerCount int
	handler     TaskHandler
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewResilientConsumer initializes a robust worker pool.
func NewResilientConsumer(broker *Broker, queueName string, workerCount int, handler TaskHandler) *ResilientConsumer {
	if workerCount <= 0 {
		workerCount = 5
	}
	return &ResilientConsumer{
		broker:      broker,
		queueName:   queueName,
		workerCount: workerCount,
		handler:     handler,
		stopChan:    make(chan struct{}),
	}
}

// Start launches the consumer loop with automatic reconnection on disconnection.
func (rc *ResilientConsumer) Start(ctx context.Context) {
	rc.wg.Add(1)
	go func() {
		defer rc.wg.Done()

		if rc.broker == nil {
			slog.Warn("Broker is nil, ResilientConsumer skipping subscription loop")
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-rc.stopChan:
				return
			default:
				deliveries, err := rc.broker.Consume(rc.queueName, rc.workerCount*2)
				if err != nil {
					slog.Warn("Failed subscribing to queue, retrying in 3s...", "error", err)
					select {
					case <-ctx.Done():
						return
					case <-rc.stopChan:
						return
					case <-time.After(3 * time.Second):
						continue
					}
				}

				rc.dispatchLoop(ctx, deliveries)
			}
		}
	}()
}

// dispatchLoop distributes incoming AMQP deliveries to a bounded pool of worker goroutines.
func (rc *ResilientConsumer) dispatchLoop(ctx context.Context, deliveries <-chan amqp.Delivery) {
	// Semaphore limiting concurrent in-flight worker executions
	sem := make(chan struct{}, rc.workerCount)

	for {
		select {
		case <-ctx.Done():
			return
		case <-rc.stopChan:
			return
		case msg, ok := <-deliveries:
			if !ok {
				slog.Warn("Delivery channel closed, triggering reconnection cycle")
				return
			}

			// Acquire worker slot
			sem <- struct{}{}

			rc.wg.Add(1)
			go func(d amqp.Delivery) {
				defer func() {
					<-sem
					rc.wg.Done()
				}()

				// Create isolated job context with timeout protection
				jobCtx, jobCancel := context.WithTimeout(ctx, 10*time.Minute)
				defer jobCancel()

				if err := rc.handler(jobCtx, d.Body); err != nil {
					attempts := getDeliveryAttempts(d)
					slog.Error("Task processing failed", "error", err, "attempts", attempts)
					if attempts >= 5 {
						slog.Error("Task reached max retries threshold, rejecting to DLQ", "attempts", attempts)
						_ = d.Nack(false, false) // Drop to dead-letter exchange (poison-message prevention)
					} else {
						_ = d.Nack(false, true) // Requeue for next retry
					}
				} else {
					_ = d.Ack(false)
				}
			}(msg)
		}
	}
}

// Stop initiates graceful drain of in-flight review tasks.
func (rc *ResilientConsumer) Stop() {
	close(rc.stopChan)
	rc.wg.Wait()
	slog.Info("Worker consumer drained and terminated safely")
}

func getDeliveryAttempts(d amqp.Delivery) int64 {
	if d.Headers != nil {
		if count, ok := d.Headers["x-delivery-count"].(int64); ok {
			return count
		}
		if count, ok := d.Headers["x-delivery-count"].(int); ok {
			return int64(count)
		}
		if deaths, ok := d.Headers["x-death"].([]any); ok && len(deaths) > 0 {
			if deathMap, ok := deaths[0].(amqp.Table); ok {
				if count, ok := deathMap["count"].(int64); ok {
					return count
				}
			}
		}
	}
	if d.Redelivered {
		return 2
	}
	return 1
}
