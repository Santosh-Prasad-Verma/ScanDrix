package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// CacheService mirrors ScanDrix CacheService providing uniform key-value caching.
type CacheService struct {
	rdb       *redis.Client
	mu        sync.RWMutex
	memory    map[string]cacheEntry
	sweeper   *time.Ticker
	stopSweep chan struct{}
}

// NewCacheService instantiates a CacheService with optional Redis client and in-memory fallback.
func NewCacheService(rdb *redis.Client) *CacheService {
	cs := &CacheService{
		rdb:       rdb,
		memory:    make(map[string]cacheEntry),
		stopSweep: make(chan struct{}),
	}
	cs.startSweeper()
	return cs
}

func (cs *CacheService) startSweeper() {
	cs.sweeper = time.NewTicker(2 * time.Minute)
	go func() {
		for {
			select {
			case <-cs.sweeper.C:
				cs.mu.Lock()
				now := time.Now()
				for k, v := range cs.memory {
					if !v.expiresAt.IsZero() && now.After(v.expiresAt) {
						delete(cs.memory, k)
					}
				}
				cs.mu.Unlock()
			case <-cs.stopSweep:
				return
			}
		}
	}()
}

// Close terminates background cleanup goroutines.
func (cs *CacheService) Close() {
	if cs.sweeper != nil {
		cs.sweeper.Stop()
	}
	close(cs.stopSweep)
}

// AddToCache serializes and stores an item with TTL (default: 60,000 ms / 1 minute).
func (cs *CacheService) AddToCache(ctx context.Context, key any, item any, ttlMs int64) error {
	if ttlMs <= 0 {
		ttlMs = 60000 // 1 minute default
	}
	keyStr := fmt.Sprintf("%v", key)
	bytes, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed marshaling cache value: %w", err)
	}

	ttlDuration := time.Duration(ttlMs) * time.Millisecond

	if cs.rdb != nil {
		return cs.rdb.Set(ctx, keyStr, string(bytes), ttlDuration).Err()
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.memory[keyStr] = cacheEntry{
		value:     string(bytes),
		expiresAt: time.Now().Add(ttlDuration),
	}
	return nil
}

// GetFromCache retrieves and unmarshals an item from cache. Returns false if not found.
func (cs *CacheService) GetFromCache(ctx context.Context, key any, target any) (bool, error) {
	keyStr := fmt.Sprintf("%v", key)

	var raw string
	if cs.rdb != nil {
		val, err := cs.rdb.Get(ctx, keyStr).Result()
		if err == redis.Nil {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		raw = val
	} else {
		cs.mu.RLock()
		entry, exists := cs.memory[keyStr]
		cs.mu.RUnlock()

		if !exists {
			return false, nil
		}
		if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
			cs.mu.Lock()
			delete(cs.memory, keyStr)
			cs.mu.Unlock()
			return false, nil
		}
		raw = entry.value
	}

	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return false, fmt.Errorf("failed unmarshaling cached JSON: %w", err)
	}
	return true, nil
}

// RemoveFromCache deletes an item from cache.
func (cs *CacheService) RemoveFromCache(ctx context.Context, key any) error {
	keyStr := fmt.Sprintf("%v", key)
	if cs.rdb != nil {
		return cs.rdb.Del(ctx, keyStr).Err()
	}

	cs.mu.Lock()
	delete(cs.memory, keyStr)
	cs.mu.Unlock()
	return nil
}

// ClearCache flushes all cached keys.
func (cs *CacheService) ClearCache(ctx context.Context) error {
	if cs.rdb != nil {
		return cs.rdb.FlushDB(ctx).Err()
	}

	cs.mu.Lock()
	cs.memory = make(map[string]cacheEntry)
	cs.mu.Unlock()
	return nil
}

// CacheExists returns true if key is present and not expired.
func (cs *CacheService) CacheExists(ctx context.Context, key any) (bool, error) {
	keyStr := fmt.Sprintf("%v", key)
	if cs.rdb != nil {
		n, err := cs.rdb.Exists(ctx, keyStr).Result()
		return n > 0, err
	}

	cs.mu.RLock()
	entry, exists := cs.memory[keyStr]
	cs.mu.RUnlock()

	if !exists {
		return false, nil
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return false, nil
	}
	return true, nil
}

// ResetCache resets the entire cache.
func (cs *CacheService) ResetCache(ctx context.Context) error {
	return cs.ClearCache(ctx)
}

// GetTTL returns the remaining TTL in milliseconds, or -1 if expired / non-existent.
func (cs *CacheService) GetTTL(ctx context.Context, key any) (int64, error) {
	keyStr := fmt.Sprintf("%v", key)
	if cs.rdb != nil {
		dur, err := cs.rdb.TTL(ctx, keyStr).Result()
		if err != nil {
			return -1, err
		}
		if dur < 0 {
			return -1, nil
		}
		return dur.Milliseconds(), nil
	}

	cs.mu.RLock()
	entry, exists := cs.memory[keyStr]
	cs.mu.RUnlock()

	if !exists {
		return -1, nil
	}
	rem := time.Until(entry.expiresAt)
	if rem <= 0 {
		return -1, nil
	}
	return rem.Milliseconds(), nil
}

// GetMultipleFromCache retrieves raw JSON strings for multiple keys in batch.
func (cs *CacheService) GetMultipleFromCache(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	if len(keys) == 0 {
		return result, nil
	}

	if cs.rdb != nil {
		vals, err := cs.rdb.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, fmt.Errorf("failed fetching multiple keys from redis: %w", err)
		}
		for i, val := range vals {
			if val != nil {
				if s, ok := val.(string); ok {
					result[keys[i]] = s
				}
			}
		}
		return result, nil
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	now := time.Now()
	for _, k := range keys {
		if entry, exists := cs.memory[k]; exists {
			if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
				result[k] = entry.value
			}
		}
	}
	return result, nil
}

// DeleteByKeyPattern deletes all cached keys matching a glob pattern (e.g. "reviews:*", "users:*").
func (cs *CacheService) DeleteByKeyPattern(ctx context.Context, pattern string) error {
	if cs.rdb != nil {
		iter := cs.rdb.Scan(ctx, 0, pattern, 0).Iterator()
		var keysToDelete []string
		for iter.Next(ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("failed scanning keys with pattern %s: %w", pattern, err)
		}
		if len(keysToDelete) > 0 {
			return cs.rdb.Del(ctx, keysToDelete...).Err()
		}
		return nil
	}

	re, err := globToRegex(pattern)
	if err != nil {
		return fmt.Errorf("invalid cache key pattern %s: %w", pattern, err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for k := range cs.memory {
		if re.MatchString(k) {
			delete(cs.memory, k)
		}
	}
	return nil
}

// globToRegex converts a Redis glob pattern to a compiled regular expression.
func globToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '[', ']', '{', '}', '\\':
			b.WriteString("\\")
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

