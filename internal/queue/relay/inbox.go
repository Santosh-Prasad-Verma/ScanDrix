package relay

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InboxDeduplicator enforces idempotent execution for incoming asynchronous consumers.
type InboxDeduplicator struct {
	mu      sync.Mutex
	records map[string]*InboxRecord // key: messageID:consumerID
}

// NewInboxDeduplicator initializes the inbox idempotency engine.
func NewInboxDeduplicator() *InboxDeduplicator {
	return &InboxDeduplicator{
		records: make(map[string]*InboxRecord),
	}
}

// ClaimMessage attempts to atomically claim a message for processing.
// Returns (true, nil) if the claim succeeded, (false, nil) if already processed or processing.
func (d *InboxDeduplicator) ClaimMessage(ctx context.Context, messageID, consumerID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	rec, exists := d.records[key]

	if exists {
		if rec.Status == InboxCompleted || rec.Status == InboxProcessing {
			return false, nil // Duplicate detected
		}
	}

	d.records[key] = &InboxRecord{
		MessageID:   messageID,
		ConsumerID:  consumerID,
		Status:      InboxProcessing,
		ProcessedAt: time.Now().UTC(),
	}
	return true, nil
}

// MarkCompleted transitions an inbox record to COMPLETED.
func (d *InboxDeduplicator) MarkCompleted(ctx context.Context, messageID, consumerID string) error {
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

// ReleaseMessage removes or resets the lock so a message can be retried.
func (d *InboxDeduplicator) ReleaseMessage(ctx context.Context, messageID, consumerID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	delete(d.records, key)
	return nil
}

