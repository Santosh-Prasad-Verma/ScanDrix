package limiter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrLockHeld         = errors.New("resource lock already held by another worker")
	ErrLockNotFound     = errors.New("lock does not exist or has expired")
	ErrLockNotOwner     = errors.New("cannot release lock owned by another worker")
)

// DistributedLockManager coordinates distributed mutual exclusion leases.
type DistributedLockManager struct {
	mu           sync.Mutex
	locks        map[string]*DistributedLock
	fenceCounter int64
}

// NewDistributedLockManager initializes the lock coordinator.
func NewDistributedLockManager() *DistributedLockManager {
	return &DistributedLockManager{
		locks: make(map[string]*DistributedLock),
	}
}

// Acquire requests an exclusive lease on a resource with automatic TTL expiration.
func (m *DistributedLockManager) Acquire(ctx context.Context, resourceKey, ownerID string, ttl time.Duration) (*DistributedLock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	existing, exists := m.locks[resourceKey]

	if exists {
		// Check if expired
		if existing.ExpiresAt.After(now) {
			if existing.OwnerID == ownerID {
				// Re-entrant extension
				existing.ExpiresAt = now.Add(ttl)
				return existing, nil
			}
			return nil, fmt.Errorf("%w: resource=%s, held_by=%s", ErrLockHeld, resourceKey, existing.OwnerID)
		}
	}

	token := atomic.AddInt64(&m.fenceCounter, 1)
	lock := &DistributedLock{
		ResourceKey:  resourceKey,
		OwnerID:      ownerID,
		FencingToken: token,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(ttl),
	}

	m.locks[resourceKey] = lock
	return lock, nil
}

// Release relinquishes the lock if held by the caller.
func (m *DistributedLockManager) Release(ctx context.Context, resourceKey, ownerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.locks[resourceKey]
	if !exists {
		return ErrLockNotFound
	}

	if existing.ExpiresAt.Before(time.Now()) {
		delete(m.locks, resourceKey)
		return ErrLockNotFound
	}

	if existing.OwnerID != ownerID {
		return fmt.Errorf("%w: expected %s, got %s", ErrLockNotOwner, existing.OwnerID, ownerID)
	}

	delete(m.locks, resourceKey)
	return nil
}
