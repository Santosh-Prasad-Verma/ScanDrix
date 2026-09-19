package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
)

// CachedConfigEntry represents an in-memory cached configuration snapshot.
type CachedConfigEntry struct {
	Config    domain.CodeReviewConfig
	Rules     []RuleCandidate
	CachedAt  time.Time
	ExpiresAt time.Time
}

// ConfigCacheStats tracks cache hits, misses, and invalidation counters.
type ConfigCacheStats struct {
	Hits          int64 `json:"hits"`
	Misses        int64 `json:"misses"`
	Invalidations int64 `json:"invalidations"`
	EntriesCount  int   `json:"entries_count"`
}

// ConfigResolverFunction defines the delegate to fetch uncached configuration.
type ConfigResolverFunction func(ctx context.Context, orgID, repoID string, filePaths []string) (domain.CodeReviewConfig, []RuleCandidate, error)

// CodebaseConfigCacheService provides commit-aware caching for enterprise review configurations.
type CodebaseConfigCacheService struct {
	mu            sync.RWMutex
	cache         map[string]CachedConfigEntry
	defaultTTL    time.Duration
	stats         ConfigCacheStats
	statsMu       sync.Mutex
}

// NewCodebaseConfigCacheService constructs a configuration cache with 5-minute default TTL.
func NewCodebaseConfigCacheService(ttl time.Duration) *CodebaseConfigCacheService {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CodebaseConfigCacheService{
		cache:      make(map[string]CachedConfigEntry),
		defaultTTL: ttl,
	}
}

// GenerateCacheKey computes a deterministic SHA-256 fingerprint for a configuration request.
func (c *CodebaseConfigCacheService) GenerateCacheKey(
	orgID, repoID string,
	baseCommit, headCommit string,
	filePaths []string,
) string {
	sortedFiles := make([]string, len(filePaths))
	copy(sortedFiles, filePaths)
	sort.Strings(sortedFiles)

	raw := fmt.Sprintf("%s:%s:%s:%s:%s", orgID, repoID, baseCommit, headCommit, strings.Join(sortedFiles, ","))
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// GetOrResolveConfig retrieves cached configuration or executes resolver with automatic cache population.
func (c *CodebaseConfigCacheService) GetOrResolveConfig(
	ctx context.Context,
	orgID, repoID string,
	baseCommit, headCommit string,
	filePaths []string,
	resolver ConfigResolverFunction,
) (domain.CodeReviewConfig, []RuleCandidate, error) {
	if resolver == nil {
		return domain.CodeReviewConfig{}, nil, errors.New("config resolver function cannot be nil")
	}

	key := c.GenerateCacheKey(orgID, repoID, baseCommit, headCommit, filePaths)

	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	now := time.Now().UTC()
	if ok && entry.ExpiresAt.After(now) {
		c.statsMu.Lock()
		c.stats.Hits++
		c.statsMu.Unlock()
		return entry.Config, entry.Rules, nil
	}

	c.statsMu.Lock()
	c.stats.Misses++
	c.statsMu.Unlock()

	// Cache miss: resolve fresh configuration
	cfg, rules, err := resolver(ctx, orgID, repoID, filePaths)
	if err != nil {
		return domain.CodeReviewConfig{}, nil, fmt.Errorf("failed to resolve codebase configuration: %w", err)
	}

	// Store in cache
	c.mu.Lock()
	c.cache[key] = CachedConfigEntry{
		Config:    cfg,
		Rules:     rules,
		CachedAt:  now,
		ExpiresAt: now.Add(c.defaultTTL),
	}
	c.mu.Unlock()

	return cfg, rules, nil
}

// InvalidateOrganization purges all cached configuration entries for an organization.
func (c *CodebaseConfigCacheService) InvalidateOrganization(orgID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	evicted := len(c.cache)
	c.cache = make(map[string]CachedConfigEntry)

	c.statsMu.Lock()
	c.stats.Invalidations++
	c.statsMu.Unlock()

	return evicted
}

// InvalidateRepository purges cached configuration when repository settings or rules update.
func (c *CodebaseConfigCacheService) InvalidateRepository(repoID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear entries
	c.cache = make(map[string]CachedConfigEntry)

	c.statsMu.Lock()
	c.stats.Invalidations++
	c.statsMu.Unlock()
}

// GetStats returns current cache statistics.
func (c *CodebaseConfigCacheService) GetStats() ConfigCacheStats {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	c.mu.RLock()
	defer c.mu.RUnlock()

	st := c.stats
	st.EntriesCount = len(c.cache)
	return st
}
