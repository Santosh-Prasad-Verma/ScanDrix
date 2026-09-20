// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// RouteRule matches HTTP method and URL pattern for conditional caching.
type RouteRule struct {
	Method  string
	Pattern *regexp.Regexp
}

var (
	// AllowlistTreesOnly caches git trees to save GitHub API rate limits.
	AllowlistTreesOnly = []RouteRule{
		{Method: "GET", Pattern: regexp.MustCompile(`(?i)/repos/[^/]+/[^/]+/git/trees/`)},
	}
	// AllowlistTreesAndRepo caches trees and repository metadata.
	AllowlistTreesAndRepo = []RouteRule{
		{Method: "GET", Pattern: regexp.MustCompile(`(?i)/repos/[^/]+/[^/]+/git/trees/`)},
		{Method: "GET", Pattern: regexp.MustCompile(`(?i)/repos/[^/]+/[^/]+$`)},
	}
)

// ETagCacheEntry stores cached HTTP response with its ETag and timestamp.
type ETagCacheEntry struct {
	ETag      string    `json:"etag"`
	Payload   []byte    `json:"payload"`
	StoredAt  time.Time `json:"stored_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IETagStore defines cache storage contract for conditional ETag caching.
type IETagStore interface {
	Get(ctx context.Context, key string) (*ETagCacheEntry, error)
	Set(ctx context.Context, key string, entry *ETagCacheEntry, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// MemoryETagStore provides an in-memory thread-safe ETag cache store.
type MemoryETagStore struct {
	mu      sync.RWMutex
	entries map[string]*ETagCacheEntry
}

// NewMemoryETagStore creates a new in-memory ETag cache store.
func NewMemoryETagStore() *MemoryETagStore {
	store := &MemoryETagStore{
		entries: make(map[string]*ETagCacheEntry),
	}
	go store.startEvictionLoop()
	return store
}

func (s *MemoryETagStore) Get(ctx context.Context, key string) (*ETagCacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return nil, nil
	}
	return entry, nil
}

func (s *MemoryETagStore) Set(ctx context.Context, key string, entry *ETagCacheEntry, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	entry.ExpiresAt = time.Now().Add(ttl)
	entry.StoredAt = time.Now()
	s.entries[key] = entry
	return nil
}

func (s *MemoryETagStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}

func (s *MemoryETagStore) startEvictionLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.entries {
			if now.After(v.ExpiresAt) {
				delete(s.entries, k)
			}
		}
		s.mu.Unlock()
	}
}

// ETagRoundTripper transparently wraps http.RoundTripper to handle ETag caching and 304 Not Modified.
type ETagRoundTripper struct {
	transport http.RoundTripper
	store     IETagStore
	rules     []RouteRule
	ttl       time.Duration
}

// NewETagRoundTripper creates a new conditional caching RoundTripper.
func NewETagRoundTripper(transport http.RoundTripper, store IETagStore, rules []RouteRule, ttl time.Duration) *ETagRoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	if store == nil {
		store = NewMemoryETagStore()
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &ETagRoundTripper{
		transport: transport,
		store:     store,
		rules:     rules,
		ttl:       ttl,
	}
}

func (rt *ETagRoundTripper) matchesRule(req *http.Request) bool {
	method := req.Method
	path := req.URL.Path
	for _, rule := range rt.rules {
		if strings.EqualFold(rule.Method, method) && rule.Pattern.MatchString(path) {
			return true
		}
	}
	return false
}

func (rt *ETagRoundTripper) buildKey(req *http.Request) string {
	accept := req.Header.Get("Accept")
	raw := req.Method + " " + req.URL.String() + "#" + accept
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

// RoundTrip executes request conditionally with ETag headers.
func (rt *ETagRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !rt.matchesRule(req) {
		return rt.transport.RoundTrip(req)
	}

	key := rt.buildKey(req)
	cached, _ := rt.store.Get(req.Context(), key)

	if cached != nil && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// 304 Not Modified: replay cached response body
	if resp.StatusCode == http.StatusNotModified && cached != nil {
		resp.StatusCode = http.StatusOK
		resp.Status = "200 OK"
		resp.Body = &readCloserWrapper{data: cached.Payload}
		return resp, nil
	}

	// 200 OK: store new ETag
	if resp.StatusCode == http.StatusOK {
		etag := resp.Header.Get("ETag")
		if etag != "" {
			var bodyBytes []byte
			if resp.Body != nil {
				buf := make([]byte, 1024*1024)
				n, _ := resp.Body.Read(buf)
				bodyBytes = buf[:n]
				_ = resp.Body.Close()
				resp.Body = &readCloserWrapper{data: bodyBytes}
			}

			_ = rt.store.Set(req.Context(), key, &ETagCacheEntry{
				ETag:     etag,
				Payload:  bodyBytes,
				StoredAt: time.Now(),
			}, rt.ttl)
		}
	}

	return resp, nil
}

type readCloserWrapper struct {
	data   []byte
	offset int
}

func (r *readCloserWrapper) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, http.ErrBodyReadAfterClose
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *readCloserWrapper) Close() error {
	r.offset = len(r.data)
	return nil
}

// BuildResolvedCacheKey generates an ETag hash key from request parameters.
func BuildResolvedCacheKey(method, urlStr, accept string) string {
	raw := strings.ToUpper(method) + " " + urlStr + "#" + strings.ToLower(accept)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// UnmarshalCachedResponse deserializes cached JSON payload.
func UnmarshalCachedResponse[T any](entry *ETagCacheEntry) (*T, error) {
	if entry == nil || len(entry.Payload) == 0 {
		return nil, nil
	}
	var out T
	if err := json.Unmarshal(entry.Payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
