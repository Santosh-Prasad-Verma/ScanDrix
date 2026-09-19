package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrLockAlreadyHeld = errors.New("distributed lock is already held by another session")
	ErrLockReleased    = errors.New("distributed lock has already been released")
)

// DistributedLockOptions configures optional auto-release TTL.
type DistributedLockOptions struct {
	TTL time.Duration
}

// DistributedLock encapsulates an acquired PostgreSQL advisory lock on a pinned connection.
type DistributedLock struct {
	conn      *pgxpool.Conn
	lockID    [2]int32
	released  bool
	ttlTimer  *time.Timer
	mu        sync.Mutex
}

// Release releases the PostgreSQL advisory lock and returns the pinned connection to the pool.
func (l *DistributedLock) Release(ctx context.Context) error {
	l.mu.Lock()
	if l.released {
		l.mu.Unlock()
		return nil
	}
	l.released = true
	if l.ttlTimer != nil {
		l.ttlTimer.Stop()
	}
	l.mu.Unlock()

	defer l.conn.Release()

	var unlocked bool
	err := l.conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1, $2)", l.lockID[0], l.lockID[1]).Scan(&unlocked)
	if err != nil {
		return fmt.Errorf("failed releasing pg_advisory_unlock: %w", err)
	}
	return nil
}

// IsReleased checks whether the lock has been released.
func (l *DistributedLock) IsReleased() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.released
}

// DistributedLockService mirrors ScanDrix DistributedLockService using PostgreSQL advisory locks.
type DistributedLockService struct {
	pool *pgxpool.Pool
}

// NewDistributedLockService instantiates a new DistributedLockService.
func NewDistributedLockService(pool *pgxpool.Pool) *DistributedLockService {
	return &DistributedLockService{pool: pool}
}

// Acquire acquires a distributed lock using PostgreSQL advisory locks on a pinned connection.
func (s *DistributedLockService) Acquire(
	ctx context.Context,
	key string,
	options DistributedLockOptions,
) (*DistributedLock, error) {
	if s.pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	lockID := HashKey(key)

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed acquiring connection for advisory lock: %w", err)
	}

	var acquired bool
	err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1, $2)", lockID[0], lockID[1]).Scan(&acquired)
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("error executing pg_try_advisory_lock: %w", err)
	}

	if !acquired {
		conn.Release()
		return nil, ErrLockAlreadyHeld
	}

	lock := &DistributedLock{
		conn:   conn,
		lockID: lockID,
	}

	if options.TTL > 0 {
		lock.ttlTimer = time.AfterFunc(options.TTL, func() {
			_ = lock.Release(context.Background())
		})
	}

	return lock, nil
}

// IsLocked checks whether an advisory lock is currently held without blocking.
func (s *DistributedLockService) IsLocked(ctx context.Context, key string) (bool, error) {
	if s.pool == nil {
		return true, errors.New("database pool not initialized")
	}

	lockID := HashKey(key)

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return true, err
	}
	defer conn.Release()

	var acquired bool
	err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1, $2)", lockID[0], lockID[1]).Scan(&acquired)
	if err != nil {
		return true, err
	}

	if acquired {
		// Immediately unlock on the same session
		var unlocked bool
		_ = conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1, $2)", lockID[0], lockID[1]).Scan(&unlocked)
		return false, nil
	}

	return true, nil
}

// HashKey hashes a string key to two 32-bit signed integers matching ScanDrix djb2 and FNV-1a algorithm.
func HashKey(key string) [2]int32 {
	// 1. djb2 hash — first 32 bits
	var hash1 int32 = 5381
	for i := 0; i < len(key); i++ {
		hash1 = ((hash1 << 5) + hash1) + int32(key[i])
	}

	// 2. FNV-1a hash — second 32 bits
	var hash2 uint32 = 0x811c9dc5
	for i := 0; i < len(key); i++ {
		hash2 ^= uint32(key[i])
		hash2 *= 0x01000193
	}

	// ScanDrix uses Math.abs(hash1) and Math.abs(hash2)
	abs1 := hash1
	if abs1 < 0 {
		abs1 = -abs1
	}
	abs2 := int32(hash2 & 0x7fffffff)

	return [2]int32{abs1, abs2}
}
