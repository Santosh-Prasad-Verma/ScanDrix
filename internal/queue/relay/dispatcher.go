package relay

import (
	"context"
	"time"
)

// MessagePublisher publishes messages to the physical message broker (RabbitMQ/Kafka).
type MessagePublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// RelayDispatcher continuously polls the outbox and publishes pending messages.
type RelayDispatcher struct {
	store     *OutboxStore
	publisher MessagePublisher
	config    RelayConfig
	stopChan  chan struct{}
}

// NewRelayDispatcher initializes the outbox relay runner.
func NewRelayDispatcher(store *OutboxStore, pub MessagePublisher, cfg RelayConfig) *RelayDispatcher {
	return &RelayDispatcher{
		store:     store,
		publisher: pub,
		config:    cfg,
		stopChan:  make(chan struct{}),
	}
}

// ProcessBatch performs a single poll-and-dispatch pass. Returns the number of messages successfully sent.
func (d *RelayDispatcher) ProcessBatch(ctx context.Context) (int, error) {
	claimed, err := d.store.ClaimPending(ctx, d.config.WorkerID, d.config.BatchSize, d.config.ClaimDuration)
	if err != nil {
		return 0, err
	}

	publishedCount := 0

	for _, msg := range claimed {
		err := d.publisher.Publish(ctx, msg.Topic, msg.Payload)
		if err != nil {
			// Record failure (increments retry count or sets DEAD_LETTER)
			_ = d.store.RecordFailure(ctx, msg.ID, err.Error())
		} else {
			// Mark message as PUBLISHED
			_ = d.store.MarkPublished(ctx, msg.ID)
			publishedCount++
		}
	}

	return publishedCount, nil
}

// Start begins background polling until context cancellation or Stop() call.
func (d *RelayDispatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-ticker.C:
			_, _ = d.ProcessBatch(ctx)
		}
	}
}

// Stop terminates the background polling loop.
func (d *RelayDispatcher) Stop() {
	close(d.stopChan)
}
