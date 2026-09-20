// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
)

type mockSuggestionStore struct {
	mu          sync.RWMutex
	suggestions map[uuid.UUID]domain.CodeSuggestion
}

func newMockSuggestionStore() *mockSuggestionStore {
	return &mockSuggestionStore{
		suggestions: make(map[uuid.UUID]domain.CodeSuggestion),
	}
}

func (m *mockSuggestionStore) GetByPR(ctx context.Context, pullNumber int) ([]domain.CodeSuggestion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.CodeSuggestion
	for _, s := range m.suggestions {
		if s.PullNumber == pullNumber {
			res = append(res, s)
		}
	}
	return res, nil
}

func (m *mockSuggestionStore) UpdateImplementationStatus(ctx context.Context, id uuid.UUID, status domain.ImplementationStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, exists := m.suggestions[id]; exists {
		s.ImplementationStatus = status
		m.suggestions[id] = s
	}
	return nil
}

type mockThreadResolver struct {
	mu             sync.Mutex
	resolvedThreads []string
}

func (r *mockThreadResolver) ResolveThread(ctx context.Context, repoID string, pullNumber int, threadID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolvedThreads = append(r.resolvedThreads, threadID)
	return nil
}

func TestImplementationVerification_ExactMatchAndAutoResolve(t *testing.T) {
	store := newMockSuggestionStore()
	resolver := &mockThreadResolver{}
	processor := NewImplementationVerificationProcessor(store, resolver)

	sugID := uuid.New()
	store.suggestions[sugID] = domain.CodeSuggestion{
		ID:                   sugID,
		PullNumber:           101,
		RelevantFile:         "pkg/db/user.go",
		DeliveryStatus:       domain.DeliveryStatusSent,
		ImplementationStatus: domain.ImplStatusNotImplemented,
		ImprovedCode:         "rows, err := db.QueryContext(ctx, query, userID)",
		Comment: &domain.SCMCommentRef{
			PlatformCommentID: "thread-404",
		},
	}

	newPatches := []pipeline.FileChangeInfo{
		{
			Filename: "pkg/db/user.go",
			Patch: `@@ -10,3 +10,4 @@
-rows, err := db.Query(query)
+rows, err := db.QueryContext(ctx, query, userID)
`,
		},
	}

	report, err := processor.VerifyCommitSuggestions(
		context.Background(),
		"repo-1",
		101,
		"commit-sha-123",
		newPatches,
	)
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}

	if report.ImplementedCount != 1 {
		t.Errorf("expected 1 implemented suggestion, got %d", report.ImplementedCount)
	}
	if report.AutoResolvedCount != 1 {
		t.Errorf("expected 1 auto-resolved thread, got %d", report.AutoResolvedCount)
	}
	if report.ImplementationRatio != 1.0 {
		t.Errorf("expected 1.0 implementation ratio, got %f", report.ImplementationRatio)
	}

	// Verify thread resolver was invoked
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if len(resolver.resolvedThreads) != 1 || resolver.resolvedThreads[0] != "thread-404" {
		t.Errorf("expected thread-404 to be resolved, got %+v", resolver.resolvedThreads)
	}
}

func TestImplementationVerification_PartialAndIgnored(t *testing.T) {
	store := newMockSuggestionStore()
	processor := NewImplementationVerificationProcessor(store)

	// Suggestion A: Partial (old flawed code removed)
	sugA := uuid.New()
	store.suggestions[sugA] = domain.CodeSuggestion{
		ID:                   sugA,
		PullNumber:           202,
		RelevantFile:         "pkg/api/handler.go",
		DeliveryStatus:       domain.DeliveryStatusSent,
		ExistingCode:         "fmt.Println(\"debug secret token\")",
		ImprovedCode:         "slog.InfoContext(ctx, \"request received\")",
		ImplementationStatus: domain.ImplStatusNotImplemented,
	}

	// Suggestion B: Ignored (touched file without adopting recommendation)
	sugB := uuid.New()
	store.suggestions[sugB] = domain.CodeSuggestion{
		ID:                   sugB,
		PullNumber:           202,
		RelevantFile:         "pkg/api/client.go",
		DeliveryStatus:       domain.DeliveryStatusSent,
		ExistingCode:         "http.Get(url)",
		ImprovedCode:         "client.Do(reqWithContext)",
		ImplementationStatus: domain.ImplStatusNotImplemented,
	}

	newPatches := []pipeline.FileChangeInfo{
		{
			Filename: "pkg/api/handler.go",
			Patch: `@@ -5,3 +5,4 @@
-fmt.Println("debug secret token")
+// removed print
`,
		},
		{
			Filename: "pkg/api/client.go",
			Patch: `@@ -12,3 +12,4 @@
+// unrelated modification
+extraVar := 1
`,
		},
	}

	report, err := processor.VerifyCommitSuggestions(
		context.Background(),
		"repo-2",
		202,
		"commit-sha-456",
		newPatches,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.PartialCount != 1 {
		t.Errorf("expected 1 partial implementation, got %d", report.PartialCount)
	}
	if report.IgnoredCount != 1 {
		t.Errorf("expected 1 ignored suggestion, got %d", report.IgnoredCount)
	}
	if report.ImplementedCount != 0 {
		t.Errorf("expected 0 fully implemented suggestions, got %d", report.ImplementedCount)
	}
}

func TestImplementationVerification_ConcurrentStress(t *testing.T) {
	store := newMockSuggestionStore()
	resolver := &mockThreadResolver{}
	processor := NewImplementationVerificationProcessor(store, resolver)

	var wg sync.WaitGroup
	workers := 25

	for w := 0; w < workers; w++ {
		id := uuid.New()
		store.suggestions[id] = domain.CodeSuggestion{
			ID:                   id,
			PullNumber:           300 + w,
			RelevantFile:         fmt.Sprintf("pkg/mod_%d/file.go", w),
			DeliveryStatus:       domain.DeliveryStatusSent,
			ImprovedCode:         "safeCall()",
			Comment:              &domain.SCMCommentRef{PlatformCommentID: fmt.Sprintf("th-%d", w)},
			ImplementationStatus: domain.ImplStatusNotImplemented,
		}
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			patches := []pipeline.FileChangeInfo{
				{
					Filename: fmt.Sprintf("pkg/mod_%d/file.go", idx),
					Patch:    "@@ -1,2 +1,3 @@\n+safeCall()\n",
				},
			}
			_, _ = processor.VerifyCommitSuggestions(
				context.Background(),
				"repo-stress",
				300+idx,
				fmt.Sprintf("sha-%d", idx),
				patches,
			)
		}(w)
	}

	wg.Wait()
}
