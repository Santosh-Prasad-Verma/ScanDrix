package relay

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InboxStore persists inbox records across distributed worker instances.
type InboxStore interface {
	ClaimInboxMessage(ctx context.Context, messageID, consumerID string) (bool, error)
	GetInboxAttemptCount(ctx context.Context, messageID, consumerID string) int
	MarkInboxCompleted(ctx context.Context, messageID, consumerID string) error
	MarkInboxFailed(ctx context.Context, messageID, consumerID string, err error) error
	ReleaseInboxMessage(ctx context.Context, messageID, consumerID string, attemptCount int) error
}

// InboxDeduplicator enforces idempotent execution for incoming asynchronous consumers.
type InboxDeduplicator struct {
	store   InboxStore
	mu      sync.Mutex
	records map[string]*InboxRecord // key: messageID:consumerID
}

// NewInboxDeduplicator initializes the inbox idempotency engine with optional persistent backend.
func NewInboxDeduplicator(store ...InboxStore) *InboxDeduplicator {
	var s InboxStore
	if len(store) > 0 {
		s = store[0]
	}
	return &InboxDeduplicator{
		store:   s,
		records: make(map[string]*InboxRecord),
	}
}

// ClaimMessage attempts to atomically claim a message for processing.
// Returns (true, nil) if the claim succeeded, (false, nil) if already processed or processing.
func (d *InboxDeduplicator) ClaimMessage(ctx context.Context, messageID, consumerID string) (bool, error) {
	if d.store != nil {
		return d.store.ClaimInboxMessage(ctx, messageID, consumerID)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	rec, exists := d.records[key]

	if exists {
		if rec.Status == InboxCompleted || rec.Status == InboxProcessing {
			return false, nil // Duplicate detected
		}
		// Message was released for retry or failed; reclaim
		rec.Status = InboxProcessing
		rec.ProcessedAt = time.Now().UTC()
		return true, nil
	}

	d.records[key] = &InboxRecord{
		MessageID:    messageID,
		ConsumerID:   consumerID,
		Status:       InboxProcessing,
		AttemptCount: 0,
		ProcessedAt:  time.Now().UTC(),
	}
	return true, nil
}

// GetAttemptCount returns the recorded attempt count for a message.
func (d *InboxDeduplicator) GetAttemptCount(messageID, consumerID string) int {
	if d.store != nil {
		return d.store.GetInboxAttemptCount(context.Background(), messageID, consumerID)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	if rec, exists := d.records[key]; exists {
		return rec.AttemptCount
	}
	return 0
}

// MarkCompleted transitions an inbox record to COMPLETED.
func (d *InboxDeduplicator) MarkCompleted(ctx context.Context, messageID, consumerID string) error {
	if d.store != nil {
		return d.store.MarkInboxCompleted(ctx, messageID, consumerID)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	rec, exists := d.records[key]
	if !exists {
		return fmt.Errorf("inbox record %s not found", key)
	}

	rec.Status = InboxCompleted
	rec.ProcessedAt = time.Now().UTC()
	return nil
}

// MarkFailed transitions an inbox record to FAILED with error context.
func (d *InboxDeduplicator) MarkFailed(ctx context.Context, messageID, consumerID string, err error) error {
	if d.store != nil {
		return d.store.MarkInboxFailed(ctx, messageID, consumerID, err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	rec, exists := d.records[key]
	if !exists {
		return fmt.Errorf("inbox record %s not found", key)
	}

	rec.Status = InboxFailed
	if err != nil {
		rec.LastError = err.Error()
	}
	return nil
}

// ReleaseMessage marks a message for retry while preserving its attempt history.
func (d *InboxDeduplicator) ReleaseMessage(ctx context.Context, messageID, consumerID string, attemptCount ...int) error {
	count := 0
	if len(attemptCount) > 0 {
		count = attemptCount[0]
	}

	if d.store != nil {
		return d.store.ReleaseInboxMessage(ctx, messageID, consumerID, count)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	if rec, exists := d.records[key]; exists {
		rec.Status = InboxRetry
		rec.AttemptCount = count
	}
	return nil
}
