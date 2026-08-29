package limiter

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// CacheStats tracks hit rates and cache effectiveness.
type CacheStats struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
	Items  int   `json:"items"`
}

// TieredCache provides thread-safe in-memory caching with TTL expiration.
type TieredCache struct {
	mu     sync.RWMutex
	items  map[string]CacheItem
	hits   int64
	misses int64
}

// NewTieredCache initializes the in-memory cache store.
func NewTieredCache() *TieredCache {
	return &TieredCache{
		items: make(map[string]CacheItem),
	}
}

// Get retrieves an item if present and not expired.
func (c *TieredCache) Get(ctx context.Context, key string) (any, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	return item.Value, true
}

// Set stores an item with the given TTL.
func (c *TieredCache) Set(ctx context.Context, key string, val any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = CacheItem{
		Value:     val,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes an item immediately.
func (c *TieredCache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// Stats returns current hit/miss metrics.
func (c *TieredCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Hits:   atomic.LoadInt64(&c.hits),
		Misses: atomic.LoadInt64(&c.misses),
		Items:  len(c.items),
	}
}
