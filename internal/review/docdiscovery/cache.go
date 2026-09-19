// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultDocTokenBudget defines the default maximum tokens allocated for documentation in reviewer prompts.
	DefaultDocTokenBudget = 2500
)

// DocSearchCache provides thread-safe caching and prompt pack assembly for third-party library documentation.
type DocSearchCache struct {
	cache map[string]DocumentationSnippet // key: packageName:version
	ttl   time.Duration
	mu    sync.RWMutex
}

// NewDocSearchCache constructs a new documentation search cache.
func NewDocSearchCache(ttl time.Duration) *DocSearchCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &DocSearchCache{
		cache: make(map[string]DocumentationSnippet),
		ttl:   ttl,
	}
}

func cacheKey(pkg, version string) string {
	return strings.ToLower(strings.TrimSpace(pkg)) + ":" + strings.TrimSpace(version)
}

// PutSnippet stores a documentation snippet in the cache.
func (c *DocSearchCache) PutSnippet(snippet DocumentationSnippet) {
	if snippet.PackageName == "" {
		return
	}
	if snippet.FetchedAt.IsZero() {
		snippet.FetchedAt = time.Now().UTC()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	k := cacheKey(snippet.PackageName, snippet.Version)
	c.cache[k] = snippet
}

// GetSnippet retrieves a cached documentation snippet if present and fresh.
func (c *DocSearchCache) GetSnippet(pkg, version string) (*DocumentationSnippet, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	k := cacheKey(pkg, version)
	s, found := c.cache[k]
	if !found {
		// Fallback without version constraint
		kFallback := cacheKey(pkg, "")
		s, found = c.cache[kFallback]
		if !found {
			return nil, false
		}
	}

	if time.Since(s.FetchedAt) > c.ttl {
		return nil, false
	}

	copied := s
	return &copied, true
}

// BuildDocumentationPack constructs a bounded documentation pack for the given dependencies.
func (c *DocSearchCache) BuildDocumentationPack(
	modifiedPackages []PackageDependency,
	maxTokens int,
) *DocumentationPack {
	if maxTokens <= 0 {
		maxTokens = DefaultDocTokenBudget
	}

	pack := &DocumentationPack{
		ModifiedPackages: modifiedPackages,
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var matchedSnippets []DocumentationSnippet
	for _, pkg := range modifiedPackages {
		if s, ok := c.GetSnippet(pkg.Name, pkg.Version); ok {
			matchedSnippets = append(matchedSnippets, *s)
		}
	}

	// Sort snippets by relevance score descending
	sort.Slice(matchedSnippets, func(i, j int) bool {
		return matchedSnippets[i].RelevanceScore > matchedSnippets[j].RelevanceScore
	})

	totalTokens := 0
	for _, s := range matchedSnippets {
		cost := s.EstimateTokens()
		if totalTokens+cost > maxTokens {
			continue
		}
		pack.Snippets = append(pack.Snippets, s)
		totalTokens += cost
	}
	pack.TotalEstimatedTokens = totalTokens

	return pack
}

// PreloadStandardLibraryDocs populates standard well-known Go, TypeScript, and Python snippets.
func (c *DocSearchCache) PreloadStandardLibraryDocs() {
	standardSnippets := []DocumentationSnippet{
		{
			PackageName:    "github.com/google/uuid",
			Version:        "v1.6.0",
			Title:          "Google UUID V7 & V4 Usage",
			URL:            "https://pkg.go.dev/github.com/google/uuid",
			Content:        "// NewV7 generates a UUIDv7 (timestamp-ordered):\nu, err := uuid.NewV7()\n// New generates random UUIDv4:\nu := uuid.New()",
			RelevanceScore: 0.9,
			FetchedAt:      time.Now().UTC(),
		},
		{
			PackageName:    "github.com/gin-gonic/gin",
			Version:        "v1.9.1",
			Title:          "Gin Engine Best Practices",
			URL:            "https://pkg.go.dev/github.com/gin-gonic/gin",
			Content:        "// Always use Recovery() and Logger() in production:\nr := gin.New()\nr.Use(gin.Recovery(), gin.Logger())",
			RelevanceScore: 0.85,
			FetchedAt:      time.Now().UTC(),
		},
		{
			PackageName:    "zod",
			Version:        "3.23.8",
			Title:          "Zod Safe Parsing",
			URL:            "https://zod.dev",
			Content:        "const result = schema.safeParse(input);\nif (!result.success) {\n  console.error(result.error);\n}",
			RelevanceScore: 0.9,
			FetchedAt:      time.Now().UTC(),
		},
	}

	for _, s := range standardSnippets {
		c.PutSnippet(s)
	}
}
