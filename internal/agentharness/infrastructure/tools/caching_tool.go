package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// CacheStats provides observability counters for tool invocation memoization.
type CacheStats struct {
	Hits   int `json:"hits"`
	Misses int `json:"misses"`
	Size   int `json:"size"`
}

// ToolCallCache provides thread-safe, run-scoped memoization for read-only tools.
type ToolCallCache struct {
	mu        sync.RWMutex
	store     map[string]contracts.ToolResult
	hitCount  int
	missCount int
}

// NewToolCallCache constructs an empty ToolCallCache.
func NewToolCallCache() *ToolCallCache {
	return &ToolCallCache{
		store: make(map[string]contracts.ToolResult),
	}
}

// Lookup retrieves a cached tool execution result.
func (c *ToolCallCache) Lookup(key string) (contracts.ToolResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	res, ok := c.store[key]
	if ok {
		c.hitCount++
	} else {
		c.missCount++
	}
	return res, ok
}

// Remember stores a successful tool execution result.
func (c *ToolCallCache) Remember(key string, result contracts.ToolResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = result
}

// Clear flushes all cached entries and resets statistics.
func (c *ToolCallCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]contracts.ToolResult)
	c.hitCount = 0
	c.missCount = 0
}

// Stats returns current hit/miss counters and cache cardinality.
func (c *ToolCallCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Hits:   c.hitCount,
		Misses: c.missCount,
		Size:   len(c.store),
	}
}

// StableStringify serializes an arbitrary structure with canonical, sorted keys
// so that equivalent dictionary keys produce identical serialization.
func StableStringify(value any) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case string:
		b, _ := json.Marshal(v)
		return string(b)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		out := "{"
		for i, k := range keys {
			if i > 0 {
				out += ","
			}
			kb, _ := json.Marshal(k)
			out += fmt.Sprintf("%s:%s", string(kb), StableStringify(v[k]))
		}
		out += "}"
		return out
	case []any:
		out := "["
		for i, item := range v {
			if i > 0 {
				out += ","
			}
			out += StableStringify(item)
		}
		out += "]"
		return out
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(b)
	}
}

// CacheKey generates a run-scoped canonical hash key.
// runID scopes the key so entries NEVER leak across runs or tenants.
func CacheKey(runID, toolName string, input any) string {
	return fmt.Sprintf("%s\x00%s\x00%s", runID, toolName, StableStringify(input))
}

// CachingTool wraps a read-only AgentTool to memoize results for the duration of a run.
// It preserves the full output body (never a dangling pointer) so that results remain
// correct even after earlier turns are truncated by context window compression.
type CachingTool struct {
	inner contracts.AgentTool
	cache *ToolCallCache
}

// NewCachingTool constructs a CachingTool decorator around an inner tool.
func NewCachingTool(inner contracts.AgentTool, cache *ToolCallCache) *CachingTool {
	if cache == nil {
		cache = NewToolCallCache()
	}
	return &CachingTool{
		inner: inner,
		cache: cache,
	}
}

func (c *CachingTool) Name() string {
	return c.inner.Name()
}

func (c *CachingTool) Description() string {
	return c.inner.Description()
}

func (c *CachingTool) InputSchema() contracts.JSONSchema {
	return c.inner.InputSchema()
}

func (c *CachingTool) Strict() bool {
	return c.inner.Strict()
}

func (c *CachingTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	key := CacheKey(ctx.RunID, c.Name(), input)

	if hit, ok := c.cache.Lookup(key); ok {
		meta := make(map[string]any, len(hit.Meta)+1)
		for k, v := range hit.Meta {
			meta[k] = v
		}
		meta["cached"] = true
		return contracts.ToolResult{
			Output:  hit.Output,
			IsError: hit.IsError,
			Meta:    meta,
		}, nil
	}

	result, err := c.inner.Execute(ctx, input)
	if err != nil {
		return result, err
	}

	// Never cache errors: transient failures must be allowed to retry.
	if !result.IsError {
		c.cache.Remember(key, result)
	}

	return result, nil
}

// WithRunCache wraps a slice of read-only tools with a shared per-run cache.
func WithRunCache(tools []contracts.AgentTool, cache *ToolCallCache) ([]contracts.AgentTool, *ToolCallCache) {
	if cache == nil {
		cache = NewToolCallCache()
	}

	wrapped := make([]contracts.AgentTool, len(tools))
	for i, t := range tools {
		wrapped[i] = NewCachingTool(t, cache)
	}
	return wrapped, cache
}
