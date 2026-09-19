package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
)

func TestPRDescriptionFitter_PlatformLimitsEnforced(t *testing.T) {
	fitter := NewPRDescriptionFitter()

	// 1. Azure DevOps (4,000 char limit)
	hugeBody := strings.Repeat("A", 5000)
	fittedADO := fitter.FitPRDescription(hugeBody, PlatformAzureDevOps)
	if len(fittedADO) > 4000 {
		t.Fatalf("expected Azure DevOps description <= 4000 chars, got %d", len(fittedADO))
	}
	if !strings.Contains(fittedADO, TruncationNotice) {
		t.Fatalf("expected truncation notice in ADO description")
	}

	// 2. Bitbucket (32,768 char limit)
	hugeBB := strings.Repeat("B", 40000)
	fittedBB := fitter.FitPRDescription(hugeBB, PlatformBitbucket)
	if len(fittedBB) > 32768 {
		t.Fatalf("expected Bitbucket description <= 32768 chars, got %d", len(fittedBB))
	}

	// 3. GitHub (65,536 char limit)
	hugeGH := strings.Repeat("G", 70000)
	fittedGH := fitter.FitPRDescription(hugeGH, PlatformGitHub)
	if len(fittedGH) > 65536 {
		t.Fatalf("expected GitHub description <= 65536 chars, got %d", len(fittedGH))
	}

	// 4. Short body under limit should be untouched
	shortBody := "Short summary under all limits"
	fittedShort := fitter.FitPRDescription(shortBody, PlatformAzureDevOps)
	if fittedShort != shortBody {
		t.Fatalf("expected short body to be unchanged, got %s", fittedShort)
	}
}

func TestPRDescriptionFitter_PreservesClosingMarker(t *testing.T) {
	fitter := NewPRDescriptionFitter()

	// Description ending with SummaryEndMarker
	content := strings.Repeat("Review detail: security vulnerability in auth. ", 200)
	bodyWithMarker := content + SummaryEndMarker

	fitted := fitter.FitPRDescription(bodyWithMarker, PlatformAzureDevOps)
	if len(fitted) > 4000 {
		t.Fatalf("expected fitted body <= 4000 chars, got %d", len(fitted))
	}
	if !strings.HasSuffix(fitted, SummaryEndMarker) {
		t.Fatalf("expected closing summary marker preserved at end, got:\n%s", fitted[len(fitted)-100:])
	}
	if !strings.Contains(fitted, TruncationNotice) {
		t.Fatalf("expected truncation notice before closing marker")
	}
}

func TestPRDescriptionFitter_ExtractPreviousSummaryAndInject(t *testing.T) {
	fitter := NewPRDescriptionFitter()

	userText := "### Developer Notes\nThis PR refactors the billing subsystem."
	oldSummary := "ScanDrix identified 3 issues: 1 critical, 2 high."
	existingBody := fmt.Sprintf("%s\n\n%s\n%s\n%s", userText, SummaryStartMarker, oldSummary, SummaryEndMarker)

	extractedSummary, extractedUserPrefix, ok := fitter.ExtractPreviousSummary(existingBody)
	if !ok {
		t.Fatalf("expected previous summary to be detected")
	}
	if extractedSummary != oldSummary {
		t.Fatalf("expected summary '%s', got '%s'", oldSummary, extractedSummary)
	}
	if extractedUserPrefix != userText {
		t.Fatalf("expected user prefix '%s', got '%s'", userText, extractedUserPrefix)
	}

	// Now inject new summary
	newSummary := "ScanDrix update: all 3 issues fixed! Clean build."
	updatedBody := fitter.InjectSummary(existingBody, newSummary, PlatformGitHub)

	if !strings.Contains(updatedBody, userText) {
		t.Fatalf("user text lost during summary injection: %s", updatedBody)
	}
	if !strings.Contains(updatedBody, newSummary) {
		t.Fatalf("new summary not injected: %s", updatedBody)
	}
	if strings.Contains(updatedBody, oldSummary) {
		t.Fatalf("old summary still present after injection: %s", updatedBody)
	}
}

func TestCodebaseConfigCacheService_HitMissAndStats(t *testing.T) {
	cache := NewCodebaseConfigCacheService(1 * time.Minute)

	var resolverCalls int32
	mockResolver := func(ctx context.Context, orgID, repoID string, filePaths []string) (domain.CodeReviewConfig, []RuleCandidate, error) {
		atomic.AddInt32(&resolverCalls, 1)
		cfg := domain.DefaultCodeReviewConfig()
		cfg.MaxSuggestions = 42
		rules := []RuleCandidate{
			{ID: "r1", Title: "Custom Rule 1"},
		}
		return cfg, rules, nil
	}

	files := []string{"src/api/auth.ts", "src/models/user.ts"}

	// 1. First call: Cache Miss
	cfg1, rules1, err := cache.GetOrResolveConfig(context.Background(), "org-1", "repo-1", "commit-a", "commit-b", files, mockResolver)
	if err != nil {
		t.Fatalf("unexpected resolver error: %v", err)
	}
	if cfg1.MaxSuggestions != 42 || len(rules1) != 1 {
		t.Fatalf("unexpected config/rules: %+v", cfg1)
	}
	if atomic.LoadInt32(&resolverCalls) != 1 {
		t.Fatalf("expected 1 resolver call, got %d", resolverCalls)
	}

	stats1 := cache.GetStats()
	if stats1.Misses != 1 || stats1.Hits != 0 || stats1.EntriesCount != 1 {
		t.Fatalf("unexpected stats on miss: %+v", stats1)
	}

	// 2. Second call with identical arguments: Cache Hit
	cfg2, rules2, err2 := cache.GetOrResolveConfig(context.Background(), "org-1", "repo-1", "commit-a", "commit-b", files, mockResolver)
	if err2 != nil {
		t.Fatalf("unexpected error on hit: %v", err2)
	}
	if cfg2.MaxSuggestions != 42 || len(rules2) != 1 {
		t.Fatalf("unexpected cached config: %+v", cfg2)
	}
	if atomic.LoadInt32(&resolverCalls) != 1 {
		t.Fatalf("resolver called again on cache hit! calls=%d", resolverCalls)
	}

	stats2 := cache.GetStats()
	if stats2.Hits != 1 || stats2.Misses != 1 {
		t.Fatalf("expected 1 hit and 1 miss, got: %+v", stats2)
	}

	// 3. File order independence in cache key
	reversedFiles := []string{"src/models/user.ts", "src/api/auth.ts"}
	cfg3, _, _ := cache.GetOrResolveConfig(context.Background(), "org-1", "repo-1", "commit-a", "commit-b", reversedFiles, mockResolver)
	if cfg3.MaxSuggestions != 42 {
		t.Fatalf("unexpected config on reversed files: %+v", cfg3)
	}
	if atomic.LoadInt32(&resolverCalls) != 1 {
		t.Fatalf("file order caused cache miss! calls=%d", resolverCalls)
	}
}

func TestCodebaseConfigCacheService_Invalidation(t *testing.T) {
	cache := NewCodebaseConfigCacheService(5 * time.Minute)

	mockResolver := func(ctx context.Context, orgID, repoID string, filePaths []string) (domain.CodeReviewConfig, []RuleCandidate, error) {
		return domain.DefaultCodeReviewConfig(), nil, nil
	}

	_, _, _ = cache.GetOrResolveConfig(context.Background(), "org-alpha", "repo-1", "c1", "c2", []string{"main.go"}, mockResolver)

	stats := cache.GetStats()
	if stats.EntriesCount != 1 {
		t.Fatalf("expected 1 cached entry, got %d", stats.EntriesCount)
	}

	// Invalidate repository
	cache.InvalidateRepository("repo-1")

	statsAfter := cache.GetStats()
	if statsAfter.EntriesCount != 0 {
		t.Fatalf("expected 0 entries after invalidation, got %d", statsAfter.EntriesCount)
	}
	if statsAfter.Invalidations != 1 {
		t.Fatalf("expected 1 invalidation recorded, got %d", statsAfter.Invalidations)
	}
}
