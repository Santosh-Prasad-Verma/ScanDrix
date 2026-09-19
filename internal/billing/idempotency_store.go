// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrEventAlreadyProcessed = errors.New("billing event has already been successfully processed")
	ErrEventCurrentlyLocked  = errors.New("billing event is currently being processed by another worker")
)

// IdempotencyStatus tracks the execution phase of a billing event.
type IdempotencyStatus string

const (
	IdempotencyStatusProcessing IdempotencyStatus = "processing"
	IdempotencyStatusCompleted  IdempotencyStatus = "completed"
	IdempotencyStatusFailed     IdempotencyStatus = "failed"
)

// IdempotencyRecord stores lock and processing details.
type IdempotencyRecord struct {
	EventID     string            `json:"event_id"`
	Status      IdempotencyStatus `json:"status"`
	LockedAt    time.Time         `json:"locked_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
}

// IdempotencyStore ensures exactly-once execution of billing hooks.
type IdempotencyStore interface {
	// TryAcquire attempts to lock the event ID for processing with a given TTL.
	// Returns true if acquired, or false if already completed or active.
	TryAcquire(ctx context.Context, eventID string, ttl time.Duration) (bool, error)
	// MarkCompleted flags the event as successfully handled.
	MarkCompleted(ctx context.Context, eventID string) error
	// MarkFailed flags the event as failed, unlocking it for potential retries.
	MarkFailed(ctx context.Context, eventID string, errReason error) error
	// CleanExpired sweeps stale expired locks.
	CleanExpired(ctx context.Context) int
}

// MemoryIdempotencyStore provides an in-memory, thread-safe idempotency registry.
type MemoryIdempotencyStore struct {
	mu      sync.RWMutex
	records map[string]IdempotencyRecord
}

// NewMemoryIdempotencyStore constructs a new in-memory idempotency cache.
func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{
		records: make(map[string]IdempotencyRecord),
	}
}

func (s *MemoryIdempotencyStore) TryAcquire(ctx context.Context, eventID string, ttl time.Duration) (bool, error) {
	if eventID == "" {
		return false, errors.New("eventID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if rec, exists := s.records[eventID]; exists {
		if rec.Status == IdempotencyStatusCompleted {
			return false, ErrEventAlreadyProcessed
		}
		if rec.Status == IdempotencyStatusProcessing && now.Before(rec.ExpiresAt) {
			return false, ErrEventCurrentlyLocked
		}
	}

	// Acquire lock
	s.records[eventID] = IdempotencyRecord{
		EventID:   eventID,
		Status:    IdempotencyStatusProcessing,
		LockedAt:  now,
		ExpiresAt: now.Add(ttl),
	}
	return true, nil
}

func (s *MemoryIdempotencyStore) MarkCompleted(ctx context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.records[eventID]
	if !exists {
		return errors.New("idempotency record not found")
	}

	now := time.Now().UTC()
	rec.Status = IdempotencyStatusCompleted
	rec.CompletedAt = &now
	rec.LastError = ""
	s.records[eventID] = rec
	return nil
}

func (s *MemoryIdempotencyStore) MarkFailed(ctx context.Context, eventID string, errReason error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.records[eventID]
	if !exists {
		return errors.New("idempotency record not found")
	}

	rec.Status = IdempotencyStatusFailed
	if errReason != nil {
		rec.LastError = errReason.Error()
	}
	// Expire immediately so retry can proceed
	rec.ExpiresAt = time.Now().UTC()
	s.records[eventID] = rec
	return nil
}

func (s *MemoryIdempotencyStore) CleanExpired(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cleaned := 0
	for id, rec := range s.records {
		if rec.Status == IdempotencyStatusProcessing && now.After(rec.ExpiresAt) {
			delete(s.records, id)
			cleaned++
		}
	}
	return cleaned
}
