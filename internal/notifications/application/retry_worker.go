package application

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/notifications/domain/contracts"
)

// NotificationRetryWorker periodically drains and re-attempts failed notifications.
type NotificationRetryWorker struct {
	deliveryRepo contracts.NotificationDeliveryRepository
	dispatcher   *NotificationDispatcherService
	workerID     string
	batchSize    int
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewNotificationRetryWorker creates a background retry worker instance.
func NewNotificationRetryWorker(
	deliveryRepo contracts.NotificationDeliveryRepository,
	dispatcher *NotificationDispatcherService,
	batchSize int,
	interval time.Duration,
) *NotificationRetryWorker {
	if batchSize <= 0 {
		batchSize = 50
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}

	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("scandrix-retry-%s-%d", hostname, os.Getpid())

	return &NotificationRetryWorker{
		deliveryRepo: deliveryRepo,
		dispatcher:   dispatcher,
		workerID:     workerID,
		batchSize:    batchSize,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the background retry polling loop.
func (w *NotificationRetryWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.drainBatches(ctx)
			}
		}
	}()
}

// Stop signals the worker to exit cleanly and waits for current drain batch to finish.
func (w *NotificationRetryWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *NotificationRetryWorker) drainBatches(ctx context.Context) {
	if w.deliveryRepo == nil || w.dispatcher == nil {
		return
	}

	for {
		batch, err := w.deliveryRepo.ClaimRetryBatch(ctx, w.batchSize, w.workerID)
		if err != nil || len(batch) == 0 {
			break
		}

		for _, delivery := range batch {
			nextAttempt := delivery.Attempts + 1
			_ = w.dispatcher.Redeliver(ctx, delivery, nextAttempt)
		}
	}
}
