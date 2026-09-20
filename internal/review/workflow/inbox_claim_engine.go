package workflow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ClaimStatus indicates the current lifecycle state of an inbox task.
type ClaimStatus string

const (
	ClaimStatusClaimed    ClaimStatus = "CLAIMED"
	ClaimStatusProcessing ClaimStatus = "PROCESSING"
	ClaimStatusCompleted  ClaimStatus = "COMPLETED"
	ClaimStatusReleased   ClaimStatus = "RELEASED"
	ClaimStatusDeadLetter ClaimStatus = "DEAD_LETTER"
	ClaimStatusExpired    ClaimStatus = "EXPIRED"
)

// InboxClaim tracks an exclusive lease on an incoming event or PR review job.
type InboxClaim struct {
	MessageID    string      `json:"message_id"`
	LockID       uuid.UUID   `json:"lock_id"`
	WorkerID     string      `json:"worker_id"`
	Status       ClaimStatus `json:"status"`
	FencingToken int64       `json:"fencing_token"`
	ClaimedAt    time.Time   `json:"claimed_at"`
	LeasedUntil  time.Time   `json:"leased_until"`
	HeartbeatAt  time.Time   `json:"heartbeat_at"`
	AttemptCount int         `json:"attempt_count"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// IInboxStore abstracts the persistence backend for distributed claims (PostgreSQL/Redis/Memory).
type IInboxStore interface {
	GetClaim(ctx context.Context, messageID string) (*InboxClaim, error)
	InsertClaim(ctx context.Context, claim *InboxClaim) error
	UpdateClaim(ctx context.Context, claim *InboxClaim) error
	DeleteClaim(ctx context.Context, messageID string) error
	ListExpired(ctx context.Context, now time.Time) ([]*InboxClaim, error)
}

// MemoryInboxStore provides a thread-safe, race-free in-memory implementation of IInboxStore.
type MemoryInboxStore struct {
	mu     sync.RWMutex
	claims map[string]*InboxClaim
}

// NewMemoryInboxStore constructs a memory inbox store.
func NewMemoryInboxStore() *MemoryInboxStore {
	return &MemoryInboxStore{
		claims: make(map[string]*InboxClaim),
	}
}

func (s *MemoryInboxStore) GetClaim(ctx context.Context, messageID string) (*InboxClaim, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.claims[messageID]
	if !ok {
		return nil, nil
	}
	copy := *c
	return &copy, nil
}

func (s *MemoryInboxStore) InsertClaim(ctx context.Context, claim *InboxClaim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.claims[claim.MessageID]; exists {
		return fmt.Errorf("claim already exists for message: %s", claim.MessageID)
	}
	copy := *claim
	s.claims[claim.MessageID] = &copy
	return nil
}

func (s *MemoryInboxStore) UpdateClaim(ctx context.Context, claim *InboxClaim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *claim
	s.claims[claim.MessageID] = &copy
	return nil
}

func (s *MemoryInboxStore) DeleteClaim(ctx context.Context, messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claims, messageID)
	return nil
}

func (s *MemoryInboxStore) ListExpired(ctx context.Context, now time.Time) ([]*InboxClaim, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var expired []*InboxClaim
	for _, c := range s.claims {
		if (c.Status == ClaimStatusClaimed || c.Status == ClaimStatusProcessing) && now.After(c.LeasedUntil) {
			copy := *c
			expired = append(expired, &copy)
		}
	}
	return expired, nil
}

// InboxClaimEngine coordinates distributed inbox message claims, heartbeats, and fencing tokens.
type InboxClaimEngine struct {
	store        IInboxStore
	defaultLease time.Duration
	tokenSeq     atomic.Int64
}

// NewInboxClaimEngine creates an inbox claim coordinator.
func NewInboxClaimEngine(store IInboxStore, defaultLease ...time.Duration) *InboxClaimEngine {
	if store == nil {
		store = NewMemoryInboxStore()
	}
	lease := 30 * time.Second
	if len(defaultLease) > 0 && defaultLease[0] > 0 {
		lease = defaultLease[0]
	}
	return &InboxClaimEngine{
		store:        store,
		defaultLease: lease,
	}
}

// TryClaim attempts to acquire an exclusive lock on an inbox message.
func (e *InboxClaimEngine) TryClaim(ctx context.Context, messageID, workerID string) (*InboxClaim, error) {
	if messageID == "" || workerID == "" {
		return nil, fmt.Errorf("messageID and workerID are required")
	}

	now := time.Now().UTC()
	existing, err := e.store.GetClaim(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed checking inbox claim: %w", err)
	}

	// 1. If existing and already completed, do not re-run (idempotency check)
	if existing != nil && existing.Status == ClaimStatusCompleted {
		return nil, fmt.Errorf("message %s already completed (idempotent skip)", messageID)
	}

	// 2. If existing and actively leased, reject concurrent attempt
	if existing != nil && (existing.Status == ClaimStatusClaimed || existing.Status == ClaimStatusProcessing) {
		if now.Before(existing.LeasedUntil) {
			return nil, fmt.Errorf("message %s is actively leased by worker %s until %v", messageID, existing.WorkerID, existing.LeasedUntil)
		}
		// Lease expired — reclaim with higher fencing token
		existing.WorkerID = workerID
		existing.Status = ClaimStatusClaimed
		existing.FencingToken = e.tokenSeq.Add(1)
		existing.HeartbeatAt = now
		existing.LeasedUntil = now.Add(e.defaultLease)
		existing.AttemptCount++
		if err := e.store.UpdateClaim(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed updating expired claim: %w", err)
		}
		return existing, nil
	}

	// 3. New Claim
	claim := &InboxClaim{
		MessageID:    messageID,
		LockID:       uuid.New(),
		WorkerID:     workerID,
		Status:       ClaimStatusClaimed,
		FencingToken: e.tokenSeq.Add(1),
		ClaimedAt:    now,
		HeartbeatAt:  now,
		LeasedUntil:  now.Add(e.defaultLease),
		AttemptCount: 1,
	}

	if existing != nil {
		claim.AttemptCount = existing.AttemptCount + 1
		if err := e.store.UpdateClaim(ctx, claim); err != nil {
			return nil, fmt.Errorf("failed updating claim: %w", err)
		}
	} else {
		if err := e.store.InsertClaim(ctx, claim); err != nil {
			return nil, fmt.Errorf("failed inserting claim: %w", err)
		}
	}

	return claim, nil
}

// RenewHeartbeat extends the lease on an actively running job, verifying the fencing token.
func (e *InboxClaimEngine) RenewHeartbeat(ctx context.Context, messageID, workerID string, fencingToken int64, extension time.Duration) error {
	claim, err := e.store.GetClaim(ctx, messageID)
	if err != nil || claim == nil {
		return fmt.Errorf("claim not found for message: %s", messageID)
	}

	if claim.WorkerID != workerID {
		return fmt.Errorf("fencing error: worker %s does not own message %s (owned by %s)", workerID, messageID, claim.WorkerID)
	}

	if claim.FencingToken != fencingToken {
		return fmt.Errorf("fencing error: obsolete token %d (current: %d)", fencingToken, claim.FencingToken)
	}

	now := time.Now().UTC()
	if extension <= 0 {
		extension = e.defaultLease
	}

	claim.HeartbeatAt = now
	claim.LeasedUntil = now.Add(extension)
	claim.Status = ClaimStatusProcessing
	return e.store.UpdateClaim(ctx, claim)
}

// ReleaseClaim completes or surrenders a claim upon task conclusion.
func (e *InboxClaimEngine) ReleaseClaim(ctx context.Context, messageID, workerID string, fencingToken int64, outcome ClaimStatus) error {
	claim, err := e.store.GetClaim(ctx, messageID)
	if err != nil || claim == nil {
		return fmt.Errorf("claim not found for message: %s", messageID)
	}

	if claim.WorkerID != workerID {
		return fmt.Errorf("worker %s does not own claim %s", workerID, messageID)
	}

	if claim.FencingToken != fencingToken {
		return fmt.Errorf("stale fencing token %d during release", fencingToken)
	}

	claim.Status = outcome
	return e.store.UpdateClaim(ctx, claim)
}

// SweepExpired releases or dead-letters abandoned claims whose heartbeat expired.
func (e *InboxClaimEngine) SweepExpired(ctx context.Context) ([]string, error) {
	now := time.Now().UTC()
	expired, err := e.store.ListExpired(ctx, now)
	if err != nil {
		return nil, err
	}

	var swept []string
	for _, c := range expired {
		c.Status = ClaimStatusExpired
		if err := e.store.UpdateClaim(ctx, c); err == nil {
			swept = append(swept, c.MessageID)
		}
	}
	return swept, nil
}
