package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// StateStore manages time-limited, one-time-use CSRF state tokens for OAuth flows.
// Each state token is valid for a configurable TTL and can only be consumed once.
// In clustered production deployments, backing by Redis ensures states are valid
// across multiple API pods without sticky sessions (Master Rule 5.1).
type StateStore struct {
	mu          sync.Mutex
	states      map[string]stateEntry
	ttl         time.Duration
	maxSize     int
	redisClient *redis.Client
}

type stateEntry struct {
	provider  OAuthProvider
	createdAt time.Time
}

// NewStateStore creates an in-memory store with the given TTL for state tokens.
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

// NewRedisStateStore creates a distributed state store backed by Redis with in-memory fallback.
func NewRedisStateStore(client *redis.Client, ttl time.Duration) *StateStore {
	store := NewStateStore(ttl)
	store.redisClient = client
	return store
}

// WithRedis attaches a Redis client to an existing StateStore.
func (s *StateStore) WithRedis(client *redis.Client) *StateStore {
	s.redisClient = client
	return s
}

// Generate creates a cryptographically random state token, stores it, and returns it.
func (s *StateStore) Generate(provider OAuthProvider) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	state := hex.EncodeToString(buf)

	if s.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := "scandrix:oauth:state:" + state
		if err := s.redisClient.Set(ctx, key, string(provider), s.ttl).Err(); err == nil {
			return state, nil
		}
		// Fallback to memory if Redis write fails
	}

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
// On success the token is consumed (deleted) atomically to prevent replay attacks.
// Returns true if valid, false otherwise.
func (s *StateStore) Validate(state string, expectedProvider OAuthProvider) bool {
	if s.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := "scandrix:oauth:state:" + state

		// Atomic GET and DEL via Lua script ensures single-use across clustered pods
		luaScript := redis.NewScript(`
			local val = redis.call('GET', KEYS[1])
			if val then
				redis.call('DEL', KEYS[1])
				return val
			else
				return nil
			end
		`)

		res, err := luaScript.Run(ctx, s.redisClient, []string{key}).Text()
		if err == nil && res != "" {
			return OAuthProvider(res) == expectedProvider
		}
		// Fallback check in local memory
	}

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
