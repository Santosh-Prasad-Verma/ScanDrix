package relay

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════
// 1. TRANSACTIONAL OUTBOX REPOSITORY (In-memory buffer & mutex)
// ═══════════════════════════════════════════════════════════════

// OutboxStore provides thread-safe outbox message persistence and lease claiming.
type OutboxStore struct {
	mu       sync.RWMutex
	messages map[uuid.UUID]*OutboxMessage
}

// NewOutboxStore initializes an empty outbox repository.
func NewOutboxStore() *OutboxStore {
	return &OutboxStore{
		messages: make(map[uuid.UUID]*OutboxMessage),
	}
}

// ═══════════════════════════════════════════════════════════════
// 2. OUTBOX EVENT INGESTION (Pending state & retry initialization)
// ═══════════════════════════════════════════════════════════════

// Insert adds a new message in PENDING state.
func (s *OutboxStore) Insert(ctx context.Context, msg OutboxMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	if msg.State == "" {
		msg.State = StatePending
	}
	if msg.MaxRetries <= 0 {
		msg.MaxRetries = 5
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	msgCopy := msg
	s.messages[msg.ID] = &msgCopy
	return nil
}

// ═══════════════════════════════════════════════════════════════
// 3. LEASE CLAIMING & DISPATCH (Worker lease locking & expiration timeout)
// ═══════════════════════════════════════════════════════════════

// ClaimPending claims up to limit pending or expired claimed messages for a worker.
func (s *OutboxStore) ClaimPending(ctx context.Context, workerID string, limit int, claimDuration time.Duration) ([]OutboxMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	expiresAt := now.Add(claimDuration)
	var claimed []OutboxMessage

	for _, msg := range s.messages {
		canClaim := false

		if msg.State == StatePending {
			canClaim = true
		} else if msg.State == StateClaimed && msg.ClaimExpiresAt != nil && msg.ClaimExpiresAt.Before(now) {
			// Lease expired, reclaimable
			canClaim = true
		} else if msg.State == StateFailed && msg.RetryCount < msg.MaxRetries {
			// Failed with retries remaining
			canClaim = true
		}

		if canClaim {
			msg.State = StateClaimed
			msg.ClaimedBy = workerID
			msg.ClaimExpiresAt = &expiresAt

			claimed = append(claimed, *msg)
			if len(claimed) >= limit {
				break
			}
		}
	}

	return claimed, nil
}

// ═══════════════════════════════════════════════════════════════
// 4. DISPATCH OUTCOME HANDLERS (Mark published or transition to dead-letter)
// ═══════════════════════════════════════════════════════════════

// MarkPublished marks a message as successfully dispatched.
func (s *OutboxStore) MarkPublished(ctx context.Context, msgID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[msgID]
	if !ok {
		return fmt.Errorf("outbox message %s not found", msgID)
	}

	now := time.Now().UTC()
	msg.State = StatePublished
	msg.PublishedAt = &now
	msg.ClaimedBy = ""
	msg.ClaimExpiresAt = nil
	return nil
}

// RecordFailure increments retry count and either schedules retry or transitions to DEAD_LETTER.
func (s *OutboxStore) RecordFailure(ctx context.Context, msgID uuid.UUID, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[msgID]
	if !ok {
		return fmt.Errorf("outbox message %s not found", msgID)
	}

	msg.RetryCount++
	msg.LastError = errMsg
	msg.ClaimedBy = ""
	msg.ClaimExpiresAt = nil

	if msg.RetryCount >= msg.MaxRetries {
		msg.State = StateDeadLetter
	} else {
		msg.State = StateFailed
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════
// 5. DEAD-LETTER REDRIVE (Resetting failed events back to pending)
// ═══════════════════════════════════════════════════════════════

// RedriveDeadLetter resets dead-lettered messages back to PENDING state.
func (s *OutboxStore) RedriveDeadLetter(ctx context.Context, topic string, limit int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, msg := range s.messages {
		if msg.State == StateDeadLetter && (topic == "" || msg.Topic == topic) {
			msg.State = StatePending
			msg.RetryCount = 0
			msg.LastError = ""
			count++
			if limit > 0 && count >= limit {
				break
			}
		}
	}
	return count, nil
}

// ═══════════════════════════════════════════════════════════════
// 6. QUEUE LAG METRICS & OBSERVABILITY (Pending age & dead-letter counts)
// ═══════════════════════════════════════════════════════════════

// GetLag calculates pending, claimed, and dead-letter statistics.
func (s *OutboxStore) GetLag(ctx context.Context) (*OutboxLagStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UTC()
	stats := &OutboxLagStats{}

	for _, msg := range s.messages {
		switch msg.State {
		case StatePending, StateFailed:
			stats.PendingCount++
			age := now.Sub(msg.CreatedAt)
			if age > stats.OldestPending {
				stats.OldestPending = age
			}
		case StateClaimed:
			stats.ClaimedCount++
		case StateDeadLetter:
			stats.DeadLetterCount++
		}
	}
	return stats, nil
}
