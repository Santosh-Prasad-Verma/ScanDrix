package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

func TestSCMThreadPublisher_GitHubBatch_Idempotency(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		if !strings.Contains(r.URL.Path, "/repos/scandrix/backend/pulls/101/comments") {
			t.Errorf("unexpected endpoint path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-gh-token" {
			t.Errorf("unexpected authorization header: %s", r.Header.Get("Authorization"))
		}

		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)

		w.Header().Set("X-RateLimit-Remaining", "995")
		w.Header().Set("X-RateLimit-Limit", "1000")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       99001,
			"html_url": "https://github.com/scandrix/backend/pull/101#discussion_r99001",
			"node_id":  "PRRC_kwDOxxxx",
		})
	}))
	defer server.Close()

	cfg := SCMThreadPublisherConfig{
		Platform:   PlatformGitHub,
		BaseURL:    server.URL,
		Token:      "test-gh-token",
		MaxRetries: 2,
	}
	pub := NewSCMThreadPublisher(cfg, server.Client())

	payloads := []PublishCommentPayload{
		{
			SuggestionID: "sug-gh-1",
			FilePath:     "pkg/auth/jwt.go",
			StartLine:    40,
			EndLine:      45,
			CommitSHA:    "abc1234",
			BaseSHA:      "base111",
			Body:         "```suggestion\nreturn token, nil\n```",
		},
	}

	// First publish call
	results, errs := pub.PublishBatch(context.Background(), "scandrix/backend", 101, payloads)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors on first publish: %v", errs)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].CommentID != "99001" {
		t.Fatalf("expected comment ID 99001, got %s", results[0].CommentID)
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 http request, got %d", requestCount)
	}

	// Second publish call with same payload (must be served from idempotency cache)
	results2, errs2 := pub.PublishBatch(context.Background(), "scandrix/backend", 101, payloads)
	if len(errs2) > 0 {
		t.Fatalf("unexpected errors on cached publish: %v", errs2)
	}
	if len(results2) != 1 || results2[0].CommentID != "99001" {
		t.Fatalf("expected cached result 99001, got %+v", results2)
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected requestCount to remain 1 due to idempotency, got %d", requestCount)
	}
}

func TestSCMThreadPublisher_GitLabDiscussionBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/projects/scandrix/backend/merge_requests/42/discussions") {
			t.Errorf("unexpected gitlab endpoint: %s", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "gl-token-xyz" {
			t.Errorf("unexpected private token: %s", r.Header.Get("PRIVATE-TOKEN"))
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "disc_7788",
			"notes": []map[string]any{
				{"id": 445566, "body": "GitLab inline suggestion"},
			},
		})
	}))
	defer server.Close()

	cfg := SCMThreadPublisherConfig{
		Platform: PlatformGitLab,
		BaseURL:  server.URL,
		Token:    "gl-token-xyz",
	}
	pub := NewSCMThreadPublisher(cfg, server.Client())

	payloads := []PublishCommentPayload{
		{
			SuggestionID: "sug-gl-1",
			FilePath:     "src/api/handler.ts",
			StartLine:    12,
			EndLine:      15,
			CommitSHA:    "head999",
			BaseSHA:      "base888",
			Body:         "```suggestion:-0+0\nconst user = auth.verify();\n```",
		},
	}

	results, errs := pub.PublishBatch(context.Background(), "scandrix/backend", 42, payloads)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors on gitlab publish: %v", errs)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ThreadID != "disc_7788" || results[0].CommentID != "445566" {
		t.Fatalf("unexpected gitlab result IDs: %+v", results[0])
	}
}

func TestSCMThreadPublisher_AzureDevOpsBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/_apis/git/repositories/repo-guid/pullRequests/12/threads") {
			t.Errorf("unexpected ado endpoint: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 5544,
			"comments": []map[string]any{
				{"id": 1},
			},
		})
	}))
	defer server.Close()

	cfg := SCMThreadPublisherConfig{
		Platform: PlatformAzureDevOps,
		BaseURL:  server.URL,
		Token:    "ado-pat-basic",
	}
	pub := NewSCMThreadPublisher(cfg, server.Client())

	payloads := []PublishCommentPayload{
		{
			SuggestionID: "sug-ado-1",
			FilePath:     "Services/UserService.cs",
			StartLine:    22,
			EndLine:      25,
			Body:         "Use asynchronous cancellation token",
			Resolution:   ThreadStatusFixed,
		},
	}

	results, errs := pub.PublishBatch(context.Background(), "repo-guid", 12, payloads)
	if len(errs) > 0 {
		t.Fatalf("unexpected ado errors: %v", errs)
	}
	if len(results) != 1 || results[0].ThreadID != "5544" {
		t.Fatalf("unexpected ado thread result: %+v", results)
	}
	if !results[0].IsResolved {
		t.Fatalf("expected ado thread result to be marked resolved")
	}
}

func TestSCMThreadPublisher_BitbucketBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repositories/scandrix/backend/pullrequests/8/comments") {
			t.Errorf("unexpected bitbucket endpoint: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 12345,
			"links": map[string]any{
				"html": map[string]any{
					"href": "https://bitbucket.org/scandrix/backend/pull-requests/8#comment-12345",
				},
			},
		})
	}))
	defer server.Close()

	cfg := SCMThreadPublisherConfig{
		Platform: PlatformBitbucket,
		BaseURL:  server.URL,
		Token:    "bb-token-abc",
	}
	pub := NewSCMThreadPublisher(cfg, server.Client())

	payloads := []PublishCommentPayload{
		{
			SuggestionID: "sug-bb-1",
			FilePath:     "app/models/user.py",
			EndLine:      55,
			Body:         "```diff\n- pass\n+ return True\n```",
		},
	}

	results, errs := pub.PublishBatch(context.Background(), "scandrix/backend", 8, payloads)
	if len(errs) > 0 {
		t.Fatalf("unexpected bb errors: %v", errs)
	}
	if len(results) != 1 || results[0].CommentID != "12345" {
		t.Fatalf("unexpected bb comment result: %+v", results)
	}
}

func TestSCMThreadPublisher_RateLimitBackoffAndJitter(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message": "rate limit exceeded"}`))
			return
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      888,
			"node_id": "NODE_OK",
		})
	}))
	defer server.Close()

	cfg := SCMThreadPublisherConfig{
		Platform:       PlatformGitHub,
		BaseURL:        server.URL,
		Token:          "test-tok",
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}
	pub := NewSCMThreadPublisher(cfg, server.Client())

	payload := PublishCommentPayload{
		SuggestionID: "sug-retry",
		FilePath:     "main.go",
		EndLine:      10,
		Body:         "fix",
	}

	res, errs := pub.PublishBatch(context.Background(), "scandrix/backend", 1, []PublishCommentPayload{payload})
	if len(errs) > 0 {
		t.Fatalf("expected successful retry, got errors: %v", errs)
	}
	if len(res) != 1 || res[0].CommentID != "888" {
		t.Fatalf("unexpected retry result: %+v", res)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", attempts)
	}
}

func TestSCMThreadPublisher_ResolveAndMinimize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"resolveReviewThread": map[string]any{"thread": map[string]any{"isResolved": true}},
				"minimizeComment":     map[string]any{"minimizedComment": map[string]any{"isMinimized": true}},
			},
		})
	}))
	defer server.Close()

	cfg := SCMThreadPublisherConfig{
		Platform: PlatformGitHub,
		BaseURL:  server.URL,
		Token:    "token-graph",
	}
	pub := NewSCMThreadPublisher(cfg, server.Client())

	if err := pub.ResolveThread(context.Background(), "scandrix/backend", 1, "THREAD_XYZ"); err != nil {
		t.Fatalf("failed to resolve thread: %v", err)
	}

	if err := pub.MinimizeComment(context.Background(), "COMMENT_NODE_XYZ", MinimizationReasonOutdated); err != nil {
		t.Fatalf("failed to minimize comment: %v", err)
	}
}

func TestConvertCodeSuggestionsToPublishPayloads(t *testing.T) {
	sugs := []*domain.CodeSuggestion{
		{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/api/router.go",
			RelevantLinesStart: 10,
			RelevantLinesEnd:   15,
			Explanation:        "Register security middleware",
		},
		{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/db/pool.go",
			RelevantLinesStart: 50,
			RelevantLinesEnd:   50,
			Explanation:        "Check pool connection error",
		},
	}

	payloads := ConvertCodeSuggestionsToPublishPayloads(sugs, "commit-head", "commit-base", func(s *domain.CodeSuggestion) string {
		return "Decorated: " + s.GetExplanation()
	})

	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if !strings.HasPrefix(payloads[0].Body, "Decorated: Register security middleware") {
		t.Fatalf("unexpected decorated body: %s", payloads[0].Body)
	}
	if payloads[1].StartLine != 50 || payloads[1].EndLine != 50 {
		t.Fatalf("unexpected line coordinates: %d-%d", payloads[1].StartLine, payloads[1].EndLine)
	}
}

// ---------------------------------------------------------------------------
// SuggestionLifecycleEngine Tests
// ---------------------------------------------------------------------------

func TestSuggestionLifecycle_ValidTransitions(t *testing.T) {
	eng := NewSuggestionLifecycleEngine()

	sug := &domain.CodeSuggestion{
		ID:           uuid.New(),
		RelevantFile: "main.go",
		StartLine:    10,
		EndLine:      12,
		Severity:     domain.SeverityMajor,
	}

	ms, err := eng.RegisterDiscoveredSuggestion(sug, "commit-1", "base-1")
	if err != nil {
		t.Fatalf("failed to register suggestion: %v", err)
	}
	if ms.CurrentState != LifecycleStateDiscovered {
		t.Fatalf("expected DISCOVERED, got %s", ms.CurrentState)
	}

	sugID := sug.ID.String()

	// DISCOVERED -> VALIDATED
	ms, err = eng.Transition(context.Background(), sugID, LifecycleStateValidated, "verifier_agent", "syntax_check_pass", "", nil)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if ms.CurrentState != LifecycleStateValidated {
		t.Fatalf("expected VALIDATED, got %s", ms.CurrentState)
	}

	// VALIDATED -> QUEUED
	ms, err = eng.Transition(context.Background(), sugID, LifecycleStateQueued, "rate_limiter", "quota_reserved", "", nil)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if ms.CurrentState != LifecycleStateQueued {
		t.Fatalf("expected QUEUED, got %s", ms.CurrentState)
	}

	// QUEUED -> PUBLISHED
	ms, err = eng.Transition(context.Background(), sugID, LifecycleStatePublished, "scm_publisher", "comment_created", "", nil)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if ms.CurrentState != LifecycleStatePublished {
		t.Fatalf("expected PUBLISHED, got %s", ms.CurrentState)
	}

	// PUBLISHED -> COMMITTED
	ms, err = eng.Transition(context.Background(), sugID, LifecycleStateCommitted, "commit_watcher", "fix_applied_in_commit", "", nil)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if ms.CurrentState != LifecycleStateCommitted {
		t.Fatalf("expected COMMITTED, got %s", ms.CurrentState)
	}

	// Audit history should contain 5 records
	if len(ms.AuditHistory) != 5 {
		t.Fatalf("expected 5 audit records, got %d", len(ms.AuditHistory))
	}
}

func TestSuggestionLifecycle_IllegalTransitionsRejected(t *testing.T) {
	eng := NewSuggestionLifecycleEngine()

	sug := &domain.CodeSuggestion{
		ID:           uuid.New(),
		RelevantFile: "main.go",
	}

	ms, _ := eng.RegisterDiscoveredSuggestion(sug, "commit-1", "base-1")
	sugID := ms.Suggestion.ID.String()

	// DISCOVERED directly to COMMITTED is illegal
	_, err := eng.Transition(context.Background(), sugID, LifecycleStateCommitted, "user", "bypass", "", nil)
	if err == nil {
		t.Fatalf("expected error on illegal transition, got nil")
	}

	// DISCOVERED to SUPERSEDED is legal
	_, err = eng.Transition(context.Background(), sugID, LifecycleStateSuperseded, "rebase", "branch_rebased", "", nil)
	if err != nil {
		t.Fatalf("expected legal transition to SUPERSEDED, got %v", err)
	}

	// SUPERSEDED is terminal -> cannot transition anywhere
	_, err = eng.Transition(context.Background(), sugID, LifecycleStateDiscovered, "user", "reopen", "", nil)
	if err == nil {
		t.Fatalf("expected error transitioning from terminal SUPERSEDED state, got nil")
	}
}

func TestSuggestionLifecycle_DrixyRuleClustering(t *testing.T) {
	eng := NewSuggestionLifecycleEngine()

	ruleID := "rule_enforce_context_first_param"
	s1 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:            uuid.New(),
			RelevantFile:  "pkg/a.go",
			BrokenRuleIDs: []string{ruleID},
			Category:      "drixy_rules",
			RankScore:     85.0,
		},
	}
	s2 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:            uuid.New(),
			RelevantFile:  "pkg/b.go",
			BrokenRuleIDs: []string{ruleID},
			Category:      "drixy_rules",
			RankScore:     95.0, // Should be elected parent
		},
	}
	s3 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:            uuid.New(),
			RelevantFile:  "pkg/c.go",
			BrokenRuleIDs: []string{ruleID},
			Category:      "drixy_rules",
			RankScore:     70.0,
		},
	}

	clustered := eng.ClusterDrixySuggestionsByRule([]*ManagedSuggestion{s1, s2, s3})
	if len(clustered) != 3 {
		t.Fatalf("expected 3 clustered items, got %d", len(clustered))
	}

	// Parent must be s2
	parent := clustered[0]
	if parent.Suggestion.ID != s2.Suggestion.ID {
		t.Fatalf("expected s2 to be elected parent due to highest rank score, got %s", parent.Suggestion.ID)
	}
	if !parent.IsParent || parent.ChildCount != 2 {
		t.Fatalf("expected parent with ChildCount 2, got IsParent=%v ChildCount=%d", parent.IsParent, parent.ChildCount)
	}

	// Children must point to parent
	parentID := parent.Suggestion.ID.String()
	for _, child := range clustered[1:] {
		if child.IsParent {
			t.Fatalf("child marked as parent")
		}
		if child.ParentSuggestionID != parentID {
			t.Fatalf("expected child parentSuggestionId=%s, got %s", parentID, child.ParentSuggestionID)
		}
		if child.PriorityStatus != PriorityStatusPrioritizedByClustering {
			t.Fatalf("expected child status PRIORITIZED_BY_CLUSTERING, got %s", child.PriorityStatus)
		}
	}
}

func TestSuggestionLifecycle_SpatialDensityDBSCAN(t *testing.T) {
	eng := NewSuggestionLifecycleEngine()

	// Suggestions in the same file: lines 10, 14 (within maxLineDistance 5)
	// and line 100 (separate cluster)
	s1 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/service.go",
			RelevantLinesStart: 10,
			RelevantLinesEnd:   12,
			RankScore:          60.0,
		},
	}
	s2 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/service.go",
			RelevantLinesStart: 14,
			RelevantLinesEnd:   16,
			RankScore:          90.0, // Elected parent of spatial cluster
		},
	}
	s3 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/service.go",
			RelevantLinesStart: 100,
			RelevantLinesEnd:   105,
			RankScore:          50.0, // Separate cluster
		},
	}

	result := eng.ClusterSuggestionsBySpatialDensity([]*ManagedSuggestion{s1, s2, s3}, 5)
	if len(result) != 3 {
		t.Fatalf("expected 3 suggestions in spatial result, got %d", len(result))
	}

	// Find the cluster parent for the line 10-16 cluster
	var clusterParent *ManagedSuggestion
	for _, ms := range result {
		if ms.IsParent && ms.Suggestion.GetStartLine() <= 16 {
			clusterParent = ms
			break
		}
	}

	if clusterParent == nil {
		t.Fatalf("expected a spatial cluster parent to be formed")
	}
	if clusterParent.Suggestion.ID != s2.Suggestion.ID {
		t.Fatalf("expected s2 to be elected parent due to rank score 90.0, got %s", clusterParent.Suggestion.ID)
	}
}

func TestSuggestionLifecycle_SeverityQuotasPartitioning(t *testing.T) {
	eng := NewSuggestionLifecycleEngine()

	makeSug := func(sev string, score float64) *ManagedSuggestion {
		return &ManagedSuggestion{
			Suggestion: &domain.CodeSuggestion{
				ID:           uuid.New(),
				RelevantFile: "main.go",
				Severity:     domain.ReviewSeverity(sev),
				RankScore:    score,
			},
		}
	}

	suggestions := []*ManagedSuggestion{
		makeSug("critical", 95.0),
		makeSug("critical", 90.0),
		makeSug("critical", 85.0), // Will be discarded (quota 2)
		makeSug("high", 80.0),
		makeSug("high", 75.0),     // Will be discarded (quota 1)
		makeSug("medium", 60.0),
		makeSug("low", 40.0),
	}

	quotas := SeverityQuotas{
		Critical: 2,
		High:     1,
		Medium:   1,
		Low:      1,
	}

	accepted, discarded := eng.PrioritizeBySeverityQuotas(suggestions, quotas)
	if len(accepted) != 5 {
		t.Fatalf("expected 5 accepted suggestions (2 crit + 1 high + 1 med + 1 low), got %d", len(accepted))
	}
	if len(discarded) != 2 {
		t.Fatalf("expected 2 discarded suggestions, got %d", len(discarded))
	}

	for _, d := range discarded {
		if d.PriorityStatus != PriorityStatusDiscardedByQuantity {
			t.Fatalf("expected DISCARDED_BY_QUANTITY, got %s", d.PriorityStatus)
		}
	}
}

func TestSuggestionLifecycle_CommitDriftHunkRebase(t *testing.T) {
	eng := NewSuggestionLifecycleEngine()

	// Suggestion at lines 50-55
	sug1 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/api/users.go",
			RelevantLinesStart: 50,
			RelevantLinesEnd:   55,
		},
	}
	// Suggestion at lines 20-25 (will be directly overwritten by diff)
	sug2 := &ManagedSuggestion{
		Suggestion: &domain.CodeSuggestion{
			ID:                 uuid.New(),
			RelevantFile:       "pkg/api/users.go",
			RelevantLinesStart: 20,
			RelevantLinesEnd:   25,
		},
	}

	// A commit added 10 lines at line 10 (shift by +10) and modified lines 20-30
	offsets := []DiffHunkOffset{
		{
			FilePath: "pkg/api/users.go",
			OldStart: 10,
			OldCount: 0,
			NewStart: 10,
			NewCount: 10,
			Delta:    10,
		},
		{
			FilePath: "pkg/api/users.go",
			OldStart: 20,
			OldCount: 10,
			NewStart: 30,
			NewCount: 15,
			Delta:    5,
		},
	}

	valid, superseded := eng.RebaseSuggestionsOnCommitDrift([]*ManagedSuggestion{sug1, sug2}, "commit-sha-2", offsets)

	if len(superseded) != 1 || superseded[0] != sug2.Suggestion.ID.String() {
		t.Fatalf("expected sug2 to be superseded due to direct overwrite, got superseded=%v", superseded)
	}

	if len(valid) != 1 {
		t.Fatalf("expected 1 valid rebased suggestion, got %d", len(valid))
	}

	rebased := valid[0].Suggestion
	// Original 50-55 shifted by delta +10 and delta +5 = +15 -> 65-70
	if rebased.GetStartLine() != 65 || rebased.GetEndLine() != 70 {
		t.Fatalf("expected rebased lines 65-70, got %d-%d", rebased.GetStartLine(), rebased.GetEndLine())
	}
}

func TestCalculateCompositeRankScore_MultiFactor(t *testing.T) {
	// Baseline critical score with AST verified and callers
	scoreCrit := CalculateCompositeRankScore("critical", true, 10, true, 1.1)
	if scoreCrit < 90.0 {
		t.Fatalf("expected high composite score for verified critical with callers, got %.2f", scoreCrit)
	}

	// Low severity without verification or callers
	scoreLow := CalculateCompositeRankScore("low", false, 0, false, 1.0)
	if scoreLow > 35.0 {
		t.Fatalf("expected low composite score for unverified low, got %.2f", scoreLow)
	}
}

// ---------------------------------------------------------------------------
// FormatSuggestionContentService Tests
// ---------------------------------------------------------------------------

func TestFormatSuggestion_CleanWhatWhyHowLabels(t *testing.T) {
	fmtService := NewFormatSuggestionContentService()

	input := `WHAT: The database handle is not closed after query execution.
WHY: This can exhaust connection pool limits under high traffic.
HOW: Add a defer db.Close() statement immediately after acquisition.`

	cleaned := fmtService.CleanWhatWhyHowLabels(input)

	if strings.Contains(cleaned, "WHAT:") || strings.Contains(cleaned, "WHY:") || strings.Contains(cleaned, "HOW:") {
		t.Fatalf("cleaned prose still contains labels: %s", cleaned)
	}
	if !strings.Contains(cleaned, "The database handle is not closed") {
		t.Fatalf("expected problem sentence to be preserved, got: %s", cleaned)
	}
	if !strings.Contains(cleaned, "defer db.Close()") {
		t.Fatalf("expected technical fix to be preserved, got: %s", cleaned)
	}
}

func TestFormatSuggestion_PlatformMarkdown_GitHub(t *testing.T) {
	fmtService := NewFormatSuggestionContentService()

	sug := &domain.CodeSuggestion{
		ID:                   uuid.New(),
		RelevantFile:         "internal/auth/token.go",
		Language:             "go",
		Severity:             domain.SeverityMajor,
		Category:             domain.ReviewCategory("security"),
		Explanation:          "Token signature check is missing algorithm validation.",
		SuggestedReplacement: "if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {\n\treturn nil, ErrInvalidAlgorithm\n}",
		Confidence:           0.95,
	}

	opts := DefaultFormattingOptions(PlatformGitHub)
	body := fmtService.BuildCommentBody(sug, opts)

	if !strings.Contains(body, "```suggestion") {
		t.Fatalf("expected github suggestion block, got: %s", body)
	}
	if !strings.Contains(body, "🟠 `High`") {
		t.Fatalf("expected High severity badge, got: %s", body)
	}
	if !strings.Contains(body, "🔒 `Security`") {
		t.Fatalf("expected Security category badge, got: %s", body)
	}
	if !strings.Contains(body, "SLSA Level 3 Provenance Verified") {
		t.Fatalf("expected SLSA posture footer, got: %s", body)
	}
}

func TestFormatSuggestion_PlatformMarkdown_Bitbucket(t *testing.T) {
	fmtService := NewFormatSuggestionContentService()

	sug := &domain.CodeSuggestion{
		ID:                   uuid.New(),
		RelevantFile:         "app/calc.py",
		Language:             "python",
		Severity:             domain.SeverityMinor,
		Category:             domain.ReviewCategory("bug"),
		Explanation:          "Divide by zero error when count is zero.",
		SuggestedReplacement: "return total / max(count, 1)",
	}

	opts := DefaultFormattingOptions(PlatformBitbucket)
	body := fmtService.BuildCommentBody(sug, opts)

	// Bitbucket must use diff syntax since it doesn't support ```suggestion
	if !strings.Contains(body, "```diff\n+ return total / max(count, 1)") {
		t.Fatalf("expected bitbucket diff block, got: %s", body)
	}
}

func TestFormatSuggestion_Localization_AllLocales(t *testing.T) {
	fmtService := NewFormatSuggestionContentService()

	sug := &domain.CodeSuggestion{
		ID:           uuid.New(),
		RelevantFile: "main.go",
		Severity:     domain.SeverityCritical,
		Category:     domain.ReviewCategory("security"),
		Explanation:  "SQL Injection defect detected in raw query format string.",
	}

	locales := []struct {
		loc      LanguageLocale
		expected string
	}{
		{LocaleES, "Crítico"},
		{LocaleJA, "緊急 (Critical)"},
		{LocalePT, "Crítico"},
		{LocaleDE, "Kritisch"},
		{LocaleFR, "Critique"},
		{LocaleZH, "严重"},
	}

	for _, tc := range locales {
		opts := DefaultFormattingOptions(PlatformGitHub)
		opts.Locale = tc.loc
		body := fmtService.BuildCommentBody(sug, opts)
		if !strings.Contains(body, tc.expected) {
			t.Errorf("expected localized label '%s' in locale %s, got:\n%s", tc.expected, tc.loc, body)
		}
	}
}

func TestCalculateCommentLines_FifteenLineBoundary(t *testing.T) {
	// 5-line range: StartLine should be preserved
	start := CalculateCommentStartLine(10, 15)
	end := CalculateCommentEndLine(10, 15)
	if start != 10 || end != 15 {
		t.Fatalf("expected 10-15, got %d-%d", start, end)
	}

	// 25-line range (> 15 lines): StartLine should be omitted (0) to avoid huge diff blocks
	startLarge := CalculateCommentStartLine(10, 35)
	endLarge := CalculateCommentEndLine(10, 35)
	if startLarge != 0 || endLarge != 10 {
		t.Fatalf("expected clamped start=0 and end=10 for >15 lines, got %d-%d", startLarge, endLarge)
	}
}

func TestFormatSuggestion_PromptAndParser(t *testing.T) {
	fmtService := NewFormatSuggestionContentService()

	sug := &domain.CodeSuggestion{
		RelevantFile:         "pkg/math.go",
		Language:             "go",
		Explanation:          "WHAT: Unchecked integer overflow. WHY: Can wrap around to negative numbers.",
		OriginalDiff:         "x += y",
		SuggestedReplacement: "x, err = checkedAdd(x, y)",
	}

	prompt := fmtService.BuildFormatPrompt([]*domain.CodeSuggestion{sug}, "Be extra concise.", "Spanish")
	if !strings.Contains(prompt, "Be extra concise.") {
		t.Fatalf("custom guidelines not found in prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "Spanish") {
		t.Fatalf("language instruction not found in prompt: %s", prompt)
	}

	mockModelOutput := `Here is the clean text:
` + "```json" + `
[
  {"index": 0, "suggestionContent": "Unchecked integer overflow can wrap around to negative numbers. Use checkedAdd to validate bounds."}
]
` + "```"

	parsed, ok := fmtService.ParseFormatResponse(mockModelOutput)
	if !ok || len(parsed) != 1 {
		t.Fatalf("failed to parse format response: %+v", parsed)
	}
	if !strings.Contains(parsed[0], "Unchecked integer overflow") {
		t.Fatalf("unexpected parsed content: %s", parsed[0])
	}
}

// ---------------------------------------------------------------------------
// SafeguardMultiLangEngine Tests
// ---------------------------------------------------------------------------

func TestSafeguardMultiLang_GoSymbolExtraction(t *testing.T) {
	eng := NewSafeguardMultiLangEngine()

	goCode := `package auth

import (
	"context"
	"fmt"
)

type TokenValidator struct {
	secret string
}

func ValidateUserToken(ctx context.Context, token string) (bool, error) {
	fmt.Println("validating")
	return true, nil
}
`
	index := eng.IndexFileSymbols("pkg/auth/validator.go", goCode)
	if index.Language != LangGo {
		t.Fatalf("expected LangGo, got %s", index.Language)
	}
	if !index.Imports["context"] || !index.Imports["fmt"] {
		t.Fatalf("missing expected imports: %+v", index.Imports)
	}
	if _, ok := index.Symbols["ValidateUserToken"]; !ok {
		t.Fatalf("expected ValidateUserToken symbol to be extracted")
	}
	if _, ok := index.Symbols["TokenValidator"]; !ok {
		t.Fatalf("expected TokenValidator type to be extracted")
	}
}

func TestSafeguardMultiLang_TypeScriptJavaScriptSymbolExtraction(t *testing.T) {
	eng := NewSafeguardMultiLangEngine()

	tsCode := `import { AxiosResponse } from 'axios';

export interface UserProfile {
	id: string;
	email: string;
}

export async function fetchUserData(userId: string): Promise<UserProfile> {
	console.log(userId);
	return { id: userId, email: "user@scandrix.dev" };
}
`
	index := eng.IndexFileSymbols("src/users.ts", tsCode)
	if index.Language != LangTypeScript {
		t.Fatalf("expected LangTypeScript, got %s", index.Language)
	}
	if _, ok := index.Symbols["UserProfile"]; !ok {
		t.Fatalf("expected UserProfile class/interface symbol")
	}
	if _, ok := index.Symbols["fetchUserData"]; !ok {
		t.Fatalf("expected fetchUserData function symbol")
	}
}

func TestSafeguardMultiLang_PythonSymbolExtraction(t *testing.T) {
	eng := NewSafeguardMultiLangEngine()

	pyCode := `import os
from typing import Optional

class ReportGenerator:
    def __init__(self, name: str):
        self.name = name

    def generate_pdf(self) -> bytes:
        return b"%PDF-1.4"
`
	index := eng.IndexFileSymbols("reports/generator.py", pyCode)
	if index.Language != LangPython {
		t.Fatalf("expected LangPython, got %s", index.Language)
	}
	if _, ok := index.Symbols["ReportGenerator"]; !ok {
		t.Fatalf("expected ReportGenerator class symbol")
	}
	if _, ok := index.Symbols["generate_pdf"]; !ok {
		t.Fatalf("expected generate_pdf function symbol")
	}
}

func TestSafeguardMultiLang_DelimiterBalancingErrors(t *testing.T) {
	eng := NewSafeguardMultiLangEngine()
	index := ASTFileIndex{FilePath: "test.go", Language: LangGo}

	// Unclosed brace
	badCode1 := `func test() { if x { return }`
	rep1 := eng.ValidateSuggestionReplacement(index, "", "x", badCode1)
	if rep1.IsValid {
		t.Fatalf("expected invalid report for unclosed brace")
	}

	// Unmatched closing paren
	badCode2 := `return fmt.Errorf("error %s", val))`
	rep2 := eng.ValidateSuggestionReplacement(index, "", "x", badCode2)
	if rep2.IsValid {
		t.Fatalf("expected invalid report for extra closing paren")
	}

	// Balanced code
	goodIndex := ASTFileIndex{
		FilePath: "test.go",
		Language: LangGo,
		Imports:  map[string]bool{"fmt": true},
	}
	goodCode := `if token == "" { return fmt.Errorf("missing token") }`
	repGood := eng.ValidateSuggestionReplacement(goodIndex, "", "token", goodCode)
	if !repGood.IsValid {
		t.Fatalf("expected valid report for balanced delimiters, got errors: %v", repGood.SyntaxErrors)
	}
}

func TestSafeguardMultiLang_HallucinatedSymbolDetection(t *testing.T) {
	eng := NewSafeguardMultiLangEngine()

	fileIndex := ASTFileIndex{
		FilePath: "services/billing.go",
		Language: LangGo,
		Symbols: map[string]ASTSymbol{
			"CalculateInvoice": {Name: "CalculateInvoice", Kind: "function"},
		},
		Imports: map[string]bool{
			"fmt": true,
		},
	}

	// Replacement introduces fake methods NonExistentBillingHelper and SuperSecretEngine
	hallucinatedCode := `func CalculateInvoice() {
		res := NonExistentBillingHelper.RunSuperSecretEngine()
		fmt.Println(res)
	}`

	report := eng.ValidateSuggestionReplacement(fileIndex, "", "func CalculateInvoice() {}", hallucinatedCode)
	if report.IsValid {
		t.Fatalf("expected report to be marked invalid due to multiple hallucinated symbols")
	}

	foundHelper := false
	for _, sym := range report.HallucinatedSymbols {
		if sym == "NonExistentBillingHelper" || sym == "SuperSecretEngine" {
			foundHelper = true
			break
		}
	}
	if !foundHelper {
		t.Fatalf("expected hallucinated symbols to include NonExistentBillingHelper, got: %v", report.HallucinatedSymbols)
	}
}

func TestSafeguardMultiLang_IndentationConsistency(t *testing.T) {
	eng := NewSafeguardMultiLangEngine()
	index := ASTFileIndex{FilePath: "main.go", Language: LangGo}

	tabbedFile := "func main() {\n\tx := 1\n\ty := 2\n}"
	spacedReplacement := "func main() {\n    x := 10\n    y := 20\n}"

	report := eng.ValidateSuggestionReplacement(index, tabbedFile, "", spacedReplacement)
	if report.IndentationIssue == "" {
		t.Fatalf("expected indentation mismatch issue detected for spaces in tabbed file")
	}
}

// ---------------------------------------------------------------------------
// DiscussionThreadSynchronizer Tests
// ---------------------------------------------------------------------------

func TestDiscussionThreadSynchronizer_RegisterPublishedThread(t *testing.T) {
	repo := NewInMemThreadSyncRepository()
	syncService := NewDiscussionThreadSynchronizer(repo, nil, nil, nil, nil)

	res := PublishedThreadResult{
		SuggestionID: "sug-100",
		CommentID:    "c-999",
		ThreadID:     "th-555",
		Platform:     PlatformGitHub,
	}

	state, err := syncService.RegisterPublishedThread(context.Background(), res, "scandrix/backend", 10, "main.go", 42)
	if err != nil {
		t.Fatalf("failed to register thread: %v", err)
	}
	if state.Status != ThreadSyncStatusSynced || state.ResolutionStatus != ThreadStatusActive {
		t.Fatalf("unexpected state: %+v", state)
	}

	stored, err := repo.GetThreadState(context.Background(), "th-555")
	if err != nil || stored.CommentID != "c-999" {
		t.Fatalf("failed to retrieve stored thread: %v", err)
	}
}

func TestDiscussionThreadSynchronizer_ReactionIngestion(t *testing.T) {
	repo := NewInMemThreadSyncRepository()
	syncService := NewDiscussionThreadSynchronizer(repo, nil, nil, nil, nil)

	err := syncService.IngestReaction(context.Background(), "c-999", "dev-alice", ReactionThumbsUp)
	if err != nil {
		t.Fatalf("failed to ingest reaction: %v", err)
	}

	outbox := syncService.DrainOutbox()
	if len(outbox) != 1 || outbox[0].EventType != "REACTION_RECEIVED" {
		t.Fatalf("expected reaction outbox event, got: %+v", outbox)
	}

	// Drain again should be empty
	if len(syncService.DrainOutbox()) != 0 {
		t.Fatalf("expected empty outbox after drain")
	}
}

func TestDiscussionThreadSynchronizer_ProcessInboundComment_Resolution(t *testing.T) {
	repo := NewInMemThreadSyncRepository()
	analysisEng := NewCommentAnalysisEngine()
	syncService := NewDiscussionThreadSynchronizer(repo, nil, nil, analysisEng, nil)

	// Register existing thread
	_, _ = syncService.RegisterPublishedThread(context.Background(), PublishedThreadResult{
		SuggestionID: "sug-resolved",
		CommentID:    "c-1",
		ThreadID:     "th-1",
		Platform:     PlatformGitHub,
	}, "scandrix/backend", 1, "main.go", 10)

	// Developer says "Already fixed in commit abc"
	state, err := syncService.ProcessInboundComment(
		context.Background(),
		"scandrix/backend",
		1,
		"th-1",
		"reply-c2",
		"dev-bob",
		"already fixed in commit abc1234",
	)

	if err != nil {
		t.Fatalf("failed to process inbound comment: %v", err)
	}

	if state.ResolutionStatus != ThreadStatusFixed {
		t.Fatalf("expected thread resolution to be FIXED, got %s", state.ResolutionStatus)
	}
	if len(state.AutomatedReplies) == 0 || !strings.Contains(state.AutomatedReplies[0], "Resolving this suggestion") {
		t.Fatalf("expected automated resolution reply, got: %v", state.AutomatedReplies)
	}
}

func TestDiscussionThreadSynchronizer_ReconcileOutdatedThreads(t *testing.T) {
	repo := NewInMemThreadSyncRepository()
	syncService := NewDiscussionThreadSynchronizer(repo, nil, nil, nil, nil)

	_, _ = syncService.RegisterPublishedThread(context.Background(), PublishedThreadResult{
		SuggestionID: "sug-outdated-1",
		CommentID:    "c-out-1",
		ThreadID:     "th-out-1",
		Platform:     PlatformGitHub,
	}, "scandrix/backend", 1, "auth.go", 20)

	count, err := syncService.ReconcileOutdatedThreads(context.Background(), "scandrix/backend", 1, []string{"sug-outdated-1"})
	if err != nil {
		t.Fatalf("failed to reconcile outdated threads: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 reconciled thread, got %d", count)
	}

	th, _ := repo.GetThreadState(context.Background(), "th-out-1")
	if th.Status != ThreadSyncStatusOutdatedOnSCM || th.ResolutionStatus != ThreadStatusClosed {
		t.Fatalf("expected thread marked outdated and closed, got status=%s res=%s", th.Status, th.ResolutionStatus)
	}
}
