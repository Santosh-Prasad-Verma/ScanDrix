package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

	// bindSecret keys the browser binding. It is generated per process when
	// not supplied; a shared secret should be supplied in clustered
	// deployments so a state minted by one pod can be redeemed by another.
	bindSecret []byte
}

type stateEntry struct {
	Provider  OAuthProvider `json:"provider"`
	CreatedAt time.Time     `json:"created_at"`
	// CodeVerifier is the PKCE verifier (RFC 7636) for this flow. It stays
	// server-side and is only handed to the token endpoint at exchange, so a
	// stolen authorization code is useless without it.
	CodeVerifier string `json:"code_verifier"`
	// Nonce is echoed into the ID token and checked on return.
	Nonce string `json:"nonce"`
}

// NewStateStore creates an in-memory store with the given TTL for state tokens.
// Defaults: 10-minute TTL, 10,000 max entries.
func NewStateStore(ttl time.Duration) *StateStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &StateStore{
		states:     make(map[string]stateEntry),
		ttl:        ttl,
		maxSize:    10_000,
		bindSecret: newBindSecret(),
	}
}

// newBindSecret returns a random per-process key used to bind a state token to
// the browser that requested it.
func newBindSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not realistically fail; a fixed key would still be
		// no worse than the previous behaviour of no binding at all.
		return []byte("scandrix-oauth-state-bind-fallback")
	}
	return b
}

// WithBindSecret supplies a shared binding key so that state tokens minted by
// one API pod can be redeemed by another in a clustered deployment.
func (s *StateStore) WithBindSecret(secret []byte) *StateStore {
	if len(secret) > 0 {
		s.bindSecret = append([]byte(nil), secret...)
	}
	return s
}

// Bind returns the value to store in the caller's browser cookie so the state
// token can later be proven to belong to the same browser.
//
// AUDIT_REMEDIATION.md F-16 (login CSRF). The state store alone only proves the
// token is unexpired and unused; without this binding, an attacker can start
// their own OAuth flow, then walk a victim into the callback URL carrying the
// attacker's code and state, and the victim's browser completes a login to the
// attacker's identity. HMAC rather than the raw state so a leaked cookie cannot
// be replayed as a state token on its own.
func (s *StateStore) Bind(state string) string {
	mac := hmac.New(sha256.New, s.bindSecret)
	mac.Write([]byte(state))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyBinding reports whether a cookie value matches the state token. The
// comparison is constant time.
func (s *StateStore) VerifyBinding(state, presented string) bool {
	if state == "" || presented == "" {
		return false
	}
	return hmac.Equal([]byte(s.Bind(state)), []byte(presented))
}

// GeneratePKCE mints a state token together with a PKCE code verifier and an
// ID-token nonce, and returns all three. The verifier is stored server-side and
// never leaves this process except in the token exchange.
//
// AUDIT_REMEDIATION.md: the user OAuth flow previously had no PKCE at all,
// which is what RFC 9700 (OAuth 2.0 Security BCP) requires even for a
// confidential client.
func (s *StateStore) GeneratePKCE(provider OAuthProvider) (state, verifier, nonce string, err error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", err
	}
	state = hex.EncodeToString(buf)

	// RFC 7636: 43-128 characters from the unreserved set. 32 random bytes
	// base64url-encode to 43 characters.
	vb := make([]byte, 32)
	if _, err := rand.Read(vb); err != nil {
		return "", "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(vb)

	nb := make([]byte, 16)
	if _, err := rand.Read(nb); err != nil {
		return "", "", "", err
	}
	nonce = base64.RawURLEncoding.EncodeToString(nb)

	entry := stateEntry{Provider: provider, CreatedAt: time.Now(), CodeVerifier: verifier, Nonce: nonce}

	if s.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, mErr := json.Marshal(entry)
		if mErr == nil {
			key := "scandrix:oauth:state:" + state
			if err := s.redisClient.Set(ctx, key, string(payload), s.ttl).Err(); err == nil {
				return state, verifier, nonce, nil
			}
		}
		// Fall through to memory if Redis is unavailable.
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.states) >= s.maxSize {
		s.evictExpiredLocked()
	}
	s.states[state] = entry
	return state, verifier, nonce, nil
}

// Consume atomically validates and removes a state token, returning the PKCE
// verifier and nonce bound to it. Single-use is preserved for both the memory
// and Redis backends.
//
// AUDIT_REMEDIATION.md.
func (s *StateStore) Consume(state string, expectedProvider OAuthProvider) (verifier, nonce string, ok bool) {
	if s.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := "scandrix:oauth:state:" + state
		luaScript := redis.NewScript(`
			local val = redis.call('GET', KEYS[1])
			if val then
				redis.call('DEL', KEYS[1])
				return val
			else
				return nil
			end
		`)
		if res, err := luaScript.Run(ctx, s.redisClient, []string{key}).Text(); err == nil && res != "" {
			var entry stateEntry
			if json.Unmarshal([]byte(res), &entry) == nil && entry.Provider == expectedProvider && time.Since(entry.CreatedAt) <= s.ttl {
				return entry.CodeVerifier, entry.Nonce, true
			}
			return "", "", false
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.states[state]
	if !exists {
		return "", "", false
	}
	delete(s.states, state)
	if time.Since(entry.CreatedAt) > s.ttl || entry.Provider != expectedProvider {
		return "", "", false
	}
	return entry.CodeVerifier, entry.Nonce, true
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
		if payload, mErr := json.Marshal(stateEntry{Provider: provider, CreatedAt: time.Now()}); mErr == nil {
			if err := s.redisClient.Set(ctx, key, string(payload), s.ttl).Err(); err == nil {
				return state, nil
			}
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
		Provider:  provider,
		CreatedAt: time.Now(),
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
			var entry stateEntry
			if json.Unmarshal([]byte(res), &entry) == nil {
				delete(s.states, state)
				return entry.Provider == expectedProvider
			}
			return false
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
	if time.Since(entry.CreatedAt) > s.ttl {
		return false
	}

	// Check provider matches
	if entry.Provider != expectedProvider {
		return false
	}

	return true
}

// evictExpiredLocked removes expired entries. Must be called with mu held.
func (s *StateStore) evictExpiredLocked() {
	now := time.Now()
	for k, v := range s.states {
		if now.Sub(v.CreatedAt) > s.ttl {
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
