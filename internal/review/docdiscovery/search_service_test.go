// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockSearchProvider struct {
	mu        sync.Mutex
	callCount int32
	delay     time.Duration
	results   map[string][]DocumentationSnippet
}

func (m *mockSearchProvider) Search(ctx context.Context, query string) ([]DocumentationSnippet, error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.results[query], nil
}

func TestDocumentationSearchService_BasicFallback(t *testing.T) {
	cache := NewDocSearchCache(1 * time.Hour)
	svc := NewDocumentationSearchService(nil, cache, nil)

	tasks := []DocQueryTask{
		{
			PackageName:     "github.com/google/uuid",
			Version:         "v1.6.0",
			PreviousVersion: "v1.3.0",
			Query:           "github.com/google/uuid v1.6.0",
			Priority:        10,
		},
		{
			PackageName: "axios",
			Version:     "1.7.0",
			Query:       "axios 1.7.0",
			Priority:    5,
		},
	}

	packages := []PackageDependency{
		{Name: "github.com/google/uuid", Version: "v1.6.0", PreviousVersion: "v1.3.0", ChangeKind: ChangeUpgraded},
		{Name: "axios", Version: "1.7.0", ChangeKind: ChangeAdded},
	}

	pack, err := svc.ExecuteDocumentationPlan(context.Background(), tasks, packages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pack.Snippets) != 2 {
		t.Fatalf("expected 2 snippets, got %d", len(pack.Snippets))
	}

	snippetByPkg := make(map[string]DocumentationSnippet)
	for _, s := range pack.Snippets {
		snippetByPkg[s.PackageName] = s
	}

	// Verify Go package snippet
	uuidSnippet, ok := snippetByPkg["github.com/google/uuid"]
	if !ok {
		t.Fatalf("expected snippet for github.com/google/uuid")
	}
	if uuidSnippet.URL != "https://pkg.go.dev/github.com/google/uuid@v1.6.0" {
		t.Errorf("unexpected URL: %s", uuidSnippet.URL)
	}

	// Verify NPM package snippet
	axiosSnippet, ok := snippetByPkg["axios"]
	if !ok {
		t.Fatalf("expected snippet for axios")
	}
	if axiosSnippet.URL != "https://www.npmjs.com/package/axios/v/1.7.0" {
		t.Errorf("unexpected URL: %s", axiosSnippet.URL)
	}
}

func TestDocumentationSearchService_SingleflightCoalescing(t *testing.T) {
	mock := &mockSearchProvider{
		delay: 30 * time.Millisecond,
		results: map[string][]DocumentationSnippet{
			"pgx v5.5.0": {
				{
					PackageName:    "github.com/jackc/pgx/v5",
					Version:        "v5.5.0",
					Title:          "pgx pool connection timeout",
					URL:            "https://pkg.go.dev/github.com/jackc/pgx/v5",
					Content:        "Configuring max connections and health check period.",
					RelevanceScore: 0.95,
				},
			},
		},
	}

	cache := NewDocSearchCache(1 * time.Hour)
	cfg := &SearchServiceConfig{
		MaxConcurrency: 10,
		TokenBudget:    5000,
	}
	svc := NewDocumentationSearchService(mock, cache, cfg)

	// Simulate 10 concurrent requests for the exact same package/query
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := DocQueryTask{
				PackageName: "github.com/jackc/pgx/v5",
				Version:     "v5.5.0",
				Query:       "pgx v5.5.0",
				Priority:    10,
			}
			_, _ = svc.ExecuteDocumentationPlan(context.Background(), []DocQueryTask{task}, nil)
		}()
	}

	wg.Wait()

	// Provider should only have been called ONCE due to singleflight!
	calls := atomic.LoadInt32(&mock.callCount)
	if calls != 1 {
		t.Errorf("expected exactly 1 provider call due to singleflight, got %d", calls)
	}

	// Subsequent request should hit cache
	_, _ = svc.ExecuteDocumentationPlan(context.Background(), []DocQueryTask{{
		PackageName: "github.com/jackc/pgx/v5",
		Version:     "v5.5.0",
		Query:       "pgx v5.5.0",
	}}, nil)

	callsAfter := atomic.LoadInt32(&mock.callCount)
	if callsAfter != 1 {
		t.Errorf("expected cached request to not invoke provider, got %d", callsAfter)
	}
}

func TestDocumentationSearchService_TokenBudgetCapping(t *testing.T) {
	cache := NewDocSearchCache(1 * time.Hour)
	// Very small token budget: 50 tokens
	cfg := &SearchServiceConfig{
		MaxConcurrency: 5,
		TokenBudget:    50,
	}
	svc := NewDocumentationSearchService(nil, cache, cfg)

	// Create 5 tasks
	var tasks []DocQueryTask
	for i := 0; i < 5; i++ {
		tasks = append(tasks, DocQueryTask{
			PackageName: fmt.Sprintf("pkg-%d", i),
			Version:     "1.0.0",
			Query:       fmt.Sprintf("pkg-%d 1.0.0", i),
			Priority:    10 - i,
		})
	}

	pack, err := svc.ExecuteDocumentationPlan(context.Background(), tasks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pack.TotalEstimatedTokens > 50 {
		t.Errorf("total tokens (%d) exceeded budget of 50", pack.TotalEstimatedTokens)
	}
	if len(pack.Snippets) >= 5 {
		t.Errorf("expected snippets to be capped below 5, got %d", len(pack.Snippets))
	}
}

func TestDocumentationSearchService_ConcurrentStress(t *testing.T) {
	cache := NewDocSearchCache(1 * time.Hour)
	svc := NewDocumentationSearchService(nil, cache, nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			task := DocQueryTask{
				PackageName: fmt.Sprintf("pkg-%d", workerID%5),
				Version:     "1.0.0",
				Query:       fmt.Sprintf("pkg-%d 1.0.0", workerID%5),
				Priority:    workerID,
			}
			_, _ = svc.ExecuteDocumentationPlan(context.Background(), []DocQueryTask{task}, nil)
		}(i)
	}
	wg.Wait()
}
