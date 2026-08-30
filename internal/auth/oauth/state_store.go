package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// StateStore manages time-limited, one-time-use CSRF state tokens for OAuth flows.
// Each state token is valid for a configurable TTL and can only be consumed once.
type StateStore struct {
	mu      sync.Mutex
	states  map[string]stateEntry
	ttl     time.Duration
	maxSize int
}

type stateEntry struct {
	provider  OAuthProvider
	createdAt time.Time
}

// NewStateStore creates a store with the given TTL for state tokens.
// Defaults: 10-minute TTL, 10,000 max entries.
func NewStateStore(ttl time.Duration) *StateStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &StateStore{
		states:  make(map[string]stateEntry),
		ttl:     ttl,
		maxSize: 10_000,
	}
}

// Generate creates a cryptographically random state token, stores it, and returns it.
func (s *StateStore) Generate(provider OAuthProvider) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	state := hex.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict expired entries if store is getting large
	if len(s.states) >= s.maxSize {
		s.evictExpiredLocked()
	}

	s.states[state] = stateEntry{
		provider:  provider,
		createdAt: time.Now(),
	}

	return state, nil
}

// Validate checks that a state token exists, is not expired, and matches the expected provider.
// On success the token is consumed (deleted) to prevent replay attacks.
// Returns true if valid, false otherwise.
func (s *StateStore) Validate(state string, expectedProvider OAuthProvider) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.states[state]
	if !exists {
		return false
	}

	// Always delete on validation attempt (one-time use)
	delete(s.states, state)

	// Check expiration
	if time.Since(entry.createdAt) > s.ttl {
		return false
	}

	// Check provider matches
	if entry.provider != expectedProvider {
		return false
	}

	return true
}

// evictExpiredLocked removes expired entries. Must be called with mu held.
func (s *StateStore) evictExpiredLocked() {
	now := time.Now()
	for k, v := range s.states {
		if now.Sub(v.createdAt) > s.ttl {
			delete(s.states, k)
		}
	}
}

// Size returns the current number of pending state tokens (for monitoring).
func (s *StateStore) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.states)
}
