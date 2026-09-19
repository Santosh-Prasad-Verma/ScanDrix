// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchProvider represents an external search engine client for fetching package docs.
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]DocumentationSnippet, error)
}

// SearchServiceConfig holds tuning parameters for documentation search execution.
type SearchServiceConfig struct {
	MaxConcurrency   int           `json:"max_concurrency"`
	RequestTimeout   time.Duration `json:"request_timeout"`
	MaxSnippetsPerPkg int          `json:"max_snippets_per_pkg"`
	TokenBudget      int           `json:"token_budget"`
}

// DefaultSearchServiceConfig returns safe production defaults.
func DefaultSearchServiceConfig() *SearchServiceConfig {
	return &SearchServiceConfig{
		MaxConcurrency:    5,
		RequestTimeout:    10 * time.Second,
		MaxSnippetsPerPkg: 2,
		TokenBudget:       4000,
	}
}

// inFlightCall represents a shared in-flight search operation (singleflight pattern).
type inFlightCall struct {
	wg       sync.WaitGroup
	snippets []DocumentationSnippet
	err      error
}

// DocumentationSearchService executes concurrent, throttled, and deduplicated documentation retrieval.
type DocumentationSearchService struct {
	provider   SearchProvider
	cache      *DocSearchCache
	config     *SearchServiceConfig
	inFlightMu sync.Mutex
	inFlight   map[string]*inFlightCall
	sem        chan struct{}
}

// NewDocumentationSearchService constructs a documentation search service.
func NewDocumentationSearchService(
	provider SearchProvider,
	cache *DocSearchCache,
	cfg *SearchServiceConfig,
) *DocumentationSearchService {
	if cfg == nil {
		cfg = DefaultSearchServiceConfig()
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 5
	}
	if cache == nil {
		cache = NewDocSearchCache(24 * time.Hour)
	}

	return &DocumentationSearchService{
		provider: provider,
		cache:    cache,
		config:   cfg,
		inFlight: make(map[string]*inFlightCall),
		sem:      make(chan struct{}, cfg.MaxConcurrency),
	}
}

// ExecuteDocumentationPlan processes planned documentation query tasks, executes concurrent
// searches with caching and singleflight deduplication, and returns a prioritized DocumentationPack.
func (s *DocumentationSearchService) ExecuteDocumentationPlan(
	ctx context.Context,
	tasks []DocQueryTask,
	modifiedPackages []PackageDependency,
) (*DocumentationPack, error) {
	if len(tasks) == 0 {
		return &DocumentationPack{
			ModifiedPackages: modifiedPackages,
			Snippets:         nil,
		}, nil
	}

	// Sort tasks by priority descending
	sortedTasks := make([]DocQueryTask, len(tasks))
	copy(sortedTasks, tasks)
	sort.Slice(sortedTasks, func(i, j int) bool {
		return sortedTasks[i].Priority > sortedTasks[j].Priority
	})

	var wg sync.WaitGroup
	resultsChan := make(chan []DocumentationSnippet, len(sortedTasks))
	errChan := make(chan error, len(sortedTasks))

	for _, task := range sortedTasks {
		wg.Add(1)
		go func(t DocQueryTask) {
			defer wg.Done()
			snippets, err := s.fetchWithSingleflightAndCache(ctx, t)
			if err != nil {
				errChan <- err
				return
			}
			if len(snippets) > 0 {
				resultsChan <- snippets
			}
		}(task)
	}

	wg.Wait()
	close(resultsChan)
	close(errChan)

	// Collect snippets and deduplicate by URL / Title
	var allSnippets []DocumentationSnippet
	seenSnippetKey := make(map[string]bool)

	for snips := range resultsChan {
		for _, snip := range snips {
			key := fmt.Sprintf("%s|%s", snip.PackageName, snip.Title)
			if !seenSnippetKey[key] {
				seenSnippetKey[key] = true
				allSnippets = append(allSnippets, snip)
			}
		}
	}

	// Sort snippets by relevance score descending
	sort.Slice(allSnippets, func(i, j int) bool {
		return allSnippets[i].RelevanceScore > allSnippets[j].RelevanceScore
	})

	// Apply Token Budget cap
	var budgetSnippets []DocumentationSnippet
	consumedTokens := 0

	for _, snip := range allSnippets {
		estimated := snip.EstimateTokens()
		if consumedTokens+estimated > s.config.TokenBudget {
			// Budget reached, skip remainder
			break
		}
		consumedTokens += estimated
		budgetSnippets = append(budgetSnippets, snip)
	}

	return &DocumentationPack{
		ModifiedPackages:     modifiedPackages,
		Snippets:             budgetSnippets,
		TotalEstimatedTokens: consumedTokens,
	}, nil
}

// fetchWithSingleflightAndCache coalesces concurrent requests for the same package query
// and checks the local cache prior to invoking external search providers.
func (s *DocumentationSearchService) fetchWithSingleflightAndCache(
	ctx context.Context,
	task DocQueryTask,
) ([]DocumentationSnippet, error) {
	cacheKey := fmt.Sprintf("%s:%s", task.PackageName, task.Version)

	// 1. Check local cache
	if cached, ok := s.cache.GetSnippet(task.PackageName, task.Version); ok {
		return []DocumentationSnippet{*cached}, nil
	}

	// 2. Singleflight synchronization
	s.inFlightMu.Lock()
	if call, exists := s.inFlight[cacheKey]; exists {
		s.inFlightMu.Unlock()
		call.wg.Wait()
		return call.snippets, call.err
	}

	call := &inFlightCall{}
	call.wg.Add(1)
	s.inFlight[cacheKey] = call
	s.inFlightMu.Unlock()

	// 3. Acquire concurrency semaphore
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		call.err = ctx.Err()
		call.wg.Done()
		s.cleanupInFlight(cacheKey)
		return nil, ctx.Err()
	}

	// 4. Perform search
	call.snippets, call.err = s.performSearch(ctx, task)
	call.wg.Done()

	// 5. Store in cache if successful
	if call.err == nil && len(call.snippets) > 0 {
		for _, snip := range call.snippets {
			s.cache.PutSnippet(snip)
		}
	}

	s.cleanupInFlight(cacheKey)
	return call.snippets, call.err
}

func (s *DocumentationSearchService) cleanupInFlight(key string) {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	delete(s.inFlight, key)
}

func (s *DocumentationSearchService) performSearch(
	ctx context.Context,
	task DocQueryTask,
) ([]DocumentationSnippet, error) {
	// If provider is configured, use it
	if s.provider != nil {
		reqCtx, cancel := context.WithTimeout(ctx, s.config.RequestTimeout)
		defer cancel()

		results, err := s.provider.Search(reqCtx, task.Query)
		if err == nil && len(results) > 0 {
			if len(results) > s.config.MaxSnippetsPerPkg {
				results = results[:s.config.MaxSnippetsPerPkg]
			}
			return results, nil
		}
	}

	// Fallback heuristic package registry documentation synthesis
	return s.generateFallbackSnippet(task), nil
}

func (s *DocumentationSearchService) generateFallbackSnippet(task DocQueryTask) []DocumentationSnippet {
	url := ""
	content := ""

	switch {
	case strings.Contains(task.PackageName, "github.com/") || strings.Contains(task.PackageName, "golang.org/"):
		url = fmt.Sprintf("https://pkg.go.dev/%s@%s", task.PackageName, task.Version)
		content = fmt.Sprintf("Package %s version %s standard Go module documentation.", task.PackageName, task.Version)
	case strings.HasPrefix(task.PackageName, "@") || !strings.Contains(task.PackageName, "/"):
		url = fmt.Sprintf("https://www.npmjs.com/package/%s/v/%s", task.PackageName, task.Version)
		content = fmt.Sprintf("NPM package %s version %s registry metadata.", task.PackageName, task.Version)
	default:
		url = fmt.Sprintf("https://pypi.org/project/%s/%s/", task.PackageName, task.Version)
		content = fmt.Sprintf("PyPI package %s version %s documentation reference.", task.PackageName, task.Version)
	}

	if task.PreviousVersion != "" {
		content += fmt.Sprintf(" Upgraded from previous version %s. Verify compatibility with API changes.", task.PreviousVersion)
	}

	return []DocumentationSnippet{
		{
			PackageName:    task.PackageName,
			Version:        task.Version,
			Title:          fmt.Sprintf("%s API Documentation Reference", task.PackageName),
			URL:            url,
			Content:        content,
			RelevanceScore: 0.75,
			FetchedAt:      time.Now().UTC(),
		},
	}
}
