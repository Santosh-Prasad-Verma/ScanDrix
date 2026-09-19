// Package services provides production-grade infrastructure implementations for ScanDrix code review.
package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

func TestDeepCommentManager_PrepareAndSubmitBatch(t *testing.T) {
	ctx := context.Background()
	mgr := NewDeepCommentManager(nil)
	mockAdapter := NewMockSCMPlatformAdapter()
	mgr.RegisterAdapter("github", mockAdapter)

	reviewID := uuid.New()
	workspaceID := uuid.New()
	repo := "acme/backend"
	pull := 42
	headSHA := "a1b2c3d4e5f6"
	baseSHA := "112233445566"

	sugID1 := uuid.New()
	sugID2 := uuid.New()
	sugID3 := uuid.New()

	suggestions := []domain.CodeSuggestion{
		{
			ID:                   sugID1,
			FilePath:             "main.go",
			StartLine:            10,
			EndLine:              15,
			Severity:             domain.SeverityCritical,
			Category:             domain.CategorySecurity,
			Description:          "SQL injection vulnerability via raw fmt.Sprintf query.",
			SuggestedReplacement: "db.QueryRowContext(ctx, \"SELECT id FROM users WHERE email = $1\", email)",
			Explanation:          "Parameterized queries prevent SQL injection attacks.",
			DeliveryStatus:       domain.DeliveryStatusQueued,
		},
		{
			ID:                   sugID2,
			FilePath:             "main.go",
			StartLine:            25,
			EndLine:              28,
			Severity:             domain.SeverityMinor,
			Category:             domain.CategoryStyle,
			Description:          "Redundant variable declaration.",
			SuggestedReplacement: "res := compute()",
			Explanation:          "Inlining simplifies code reading.",
			DeliveryStatus:       domain.DeliveryStatusQueued,
		},
		{
			ID:             sugID3,
			FilePath:       "main.go",
			StartLine:      40,
			EndLine:        42,
			Severity:       domain.SeverityInfo,
			Category:       domain.CategoryStyle,
			Description:    "Should not be delivered.",
			DeliveryStatus: domain.DeliveryStatusSuppressed,
		},
	}

	summaryBody := mgr.BuildReviewSummaryMarkdown(
		reviewID,
		"Feature: Add user billing",
		pull,
		"octocat",
		headSHA,
		suggestions[:2],
		150*time.Millisecond,
		"STANDARD",
	)

	if !strings.Contains(summaryBody, "CHANGES REQUIRED") {
		t.Fatalf("expected critical finding to require changes in summary, got: %s", summaryBody)
	}

	batch := mgr.PrepareReviewBatch(
		reviewID,
		workspaceID,
		repo,
		pull,
		headSHA,
		baseSHA,
		suggestions,
		summaryBody,
		"octocat",
	)

	// Suppressed suggestion must be omitted
	if len(batch.Comments) != 2 {
		t.Fatalf("expected 2 active comments, got %d", len(batch.Comments))
	}
	if batch.Event != "REQUEST_CHANGES" {
		t.Fatalf("expected critical finding to trigger REQUEST_CHANGES event, got %s", batch.Event)
	}

	submissionID, err := mgr.SubmitReviewBatch(ctx, "github", batch)
	if err != nil {
		t.Fatalf("failed to submit review batch: %v", err)
	}
	if submissionID == "" {
		t.Fatal("expected non-empty submission ID")
	}

	if len(mockAdapter.Batches) != 1 {
		t.Fatalf("expected 1 submitted batch in mock, got %d", len(mockAdapter.Batches))
	}
}

func TestDeepCommentManager_ReconcileResolvedComments(t *testing.T) {
	ctx := context.Background()
	mgr := NewDeepCommentManager(nil)
	mockAdapter := NewMockSCMPlatformAdapter()
	mgr.RegisterAdapter("github", mockAdapter)

	repo := "acme/backend"
	pull := 101

	// Seed existing threads
	mockAdapter.Existing = []CommentThreadState{
		{
			ThreadID:      "th-resolved",
			RootCommentID: "com-1",
			FilePath:      "server.go",
			Line:          30,
			Fingerprint:   "fp-resolved-defect",
			Status:        ThreadStatusActive,
		},
		{
			ThreadID:      "th-persisting",
			RootCommentID: "com-2",
			FilePath:      "server.go",
			Line:          50,
			Fingerprint:   "fp-still-present",
			Status:        ThreadStatusActive,
		},
	}

	if err := mgr.SyncExistingThreads(ctx, "github", repo, pull); err != nil {
		t.Fatalf("failed syncing existing threads: %v", err)
	}

	// Current review only has the second finding (first was fixed by developer!)
	currentSuggestions := []domain.CodeSuggestion{
		{
			ID:          uuid.New(),
			FilePath:    "server.go",
			StartLine:   50,
			EndLine:     52,
			Fingerprint: "fp-still-present",
		},
	}

	resolvedCount, err := mgr.ReconcileResolvedComments(ctx, "github", repo, pull, currentSuggestions)
	if err != nil {
		t.Fatalf("reconciliation failed: %v", err)
	}

	if resolvedCount != 1 {
		t.Fatalf("expected 1 comment to be resolved, got %d", resolvedCount)
	}

	if mockAdapter.Resolved["th-resolved"] != ThreadStatusFixed {
		t.Fatalf("expected th-resolved status to be FIXED, got %s", mockAdapter.Resolved["th-resolved"])
	}
	if mockAdapter.Minimized["com-1"] != MinimizationReasonResolved {
		t.Fatalf("expected root comment to be minimized as RESOLVED, got %s", mockAdapter.Minimized["com-1"])
	}
}

func TestDeepSuggestionService_FilteringAndLimits(t *testing.T) {
	svc := NewDeepSuggestionService(nil, nil)

	sugID1 := uuid.New()
	sugID2 := uuid.New()
	sugID3 := uuid.New()

	sugs := []domain.CodeSuggestion{
		{
			ID:          sugID1,
			FilePath:    "vendor/lib.go",
			StartLine:   10,
			EndLine:     12,
			Severity:    domain.SeverityCritical,
			Category:    domain.CategorySecurity,
			Description: "Vendor defect",
		},
		{
			ID:          sugID2,
			FilePath:    "pkg/auth/token.go",
			StartLine:   20,
			EndLine:     25,
			Severity:    domain.SeverityCritical,
			Category:    domain.CategorySecurity,
			Description: "Hardcoded secret key",
			Confidence:  0.95,
		},
		{
			ID:          sugID3,
			FilePath:    "pkg/auth/token.go",
			StartLine:   35,
			EndLine:     37,
			Severity:    domain.SeverityInfo,
			Category:    domain.CategoryStyle,
			Description: "Variable name could be shorter",
			Confidence:  0.4,
		},
	}

	cfg := domain.CodeReviewConfig{
		Strictness:     domain.StrictnessBalanced,
		IgnorePatterns: []string{"vendor/**"},
	}

	kept, discarded := svc.FilterSuggestionsByReviewOptions(sugs, cfg)
	if len(kept) != 1 {
		t.Fatalf("expected 1 kept suggestion, got %d", len(kept))
	}
	if kept[0].ID != sugID2 {
		t.Fatalf("expected sugID2 to be kept, got %s", kept[0].ID)
	}
	if len(discarded) != 2 {
		t.Fatalf("expected 2 discarded suggestions, got %d", len(discarded))
	}

	// Diff boundary filtering
	hunks := []DiffLineHunk{
		{
			FilePath:  "pkg/auth/token.go",
			NewStart:  15,
			NewLength: 15, // covers lines 15..29
		},
	}

	diffKept, diffDiscarded := svc.FilterSuggestionsCodeDiff(kept, hunks)
	if len(diffKept) != 1 || diffKept[0].ID != sugID2 {
		t.Fatalf("expected sugID2 to stay within diff hunk, kept: %d, discarded: %d", len(diffKept), len(diffDiscarded))
	}

	// Severity limits check
	limits := SeverityLimitsConfig{
		MaxCritical: 1,
		MaxMajor:    1,
		MaxMinor:    1,
		MaxInfo:     1,
		MaxTotal:    2,
	}

	cID1 := uuid.New()
	cID2 := uuid.New()

	critSugs := []domain.CodeSuggestion{
		{ID: cID1, FilePath: "a.go", StartLine: 1, Severity: domain.SeverityCritical, Confidence: 0.9},
		{ID: cID2, FilePath: "b.go", StartLine: 2, Severity: domain.SeverityCritical, Confidence: 0.5},
	}

	prioKept, prioDiscarded := svc.PrioritizeSuggestionsBySeverityLimits(critSugs, limits)
	if len(prioKept) != 1 || prioKept[0].ID != cID1 {
		t.Fatalf("expected cID1 to win on confidence/score, got %v", prioKept)
	}
	if len(prioDiscarded) != 1 {
		t.Fatalf("expected cID2 to be discarded due to quota, got %d", len(prioDiscarded))
	}
}

func TestDeepSuggestionService_ClusteringAndRebasing(t *testing.T) {
	svc := NewDeepSuggestionService(nil, nil)

	sugs := []domain.CodeSuggestion{
		{
			ID:        uuid.New(),
			FilePath:  "api.go",
			StartLine: 10,
			EndLine:   12,
			Category:  domain.CategorySecurity,
			Severity:  domain.SeverityMajor,
		},
		{
			ID:        uuid.New(),
			FilePath:  "api.go",
			StartLine: 15,
			EndLine:   18,
			Category:  domain.CategorySecurity,
			Severity:  domain.SeverityCritical,
		},
		{
			ID:        uuid.New(),
			FilePath:  "api.go",
			StartLine: 80,
			EndLine:   85,
			Category:  domain.CategorySecurity,
			Severity:  domain.SeverityMinor,
		},
	}

	// Line proximity threshold 10: s1 (10-12) and s2 (15-18) are 3 lines apart -> should cluster!
	clusters := svc.ClusterRelatedSuggestions(sugs, 10)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
	if len(clusters[0].Suggestions) != 2 {
		t.Fatalf("expected first cluster to have 2 suggestions, got %d", len(clusters[0].Suggestions))
	}
	if clusters[0].Severity != domain.SeverityCritical {
		t.Fatalf("expected cluster severity to be promoted to CRITICAL, got %s", clusters[0].Severity)
	}

	// Line coordinate rebasing
	deltas := []LineDelta{
		{
			FilePath:   "api.go",
			LineNumber: 5,
			Delta:      10, // 10 lines inserted before line 10
		},
	}

	rebased := svc.RebaseSuggestionLineNumbers(sugs, deltas)
	if rebased[0].StartLine != 20 || rebased[0].EndLine != 22 {
		t.Fatalf("expected s1 to shift to 20-22, got %d-%d", rebased[0].StartLine, rebased[0].EndLine)
	}
}

func TestSafeguardPipelineEngine_TriageAndHallucination(t *testing.T) {
	eng := NewSafeguardPipelineEngine(nil)

	// Hard discard test: quality opinion
	f1 := SafeguardFeatureSet{
		IsQualityOpinion:      true,
		HasResourceLeak:       true,
		ImprovedCodeIsCorrect: true,
	}
	if dec := eng.TriageSuggestion(f1); dec != TriageDecisionDiscard {
		t.Fatalf("expected hard discard for quality opinion, got %s", dec)
	}

	// Soft speculation + structural defect -> verify
	f2 := SafeguardFeatureSet{
		RequiresAssumedInput:  true,
		HasDataExposure:       true,
		ImprovedCodeIsCorrect: true,
	}
	if dec := eng.TriageSuggestion(f2); dec != TriageDecisionVerify {
		t.Fatalf("expected verify for soft speculation + defect, got %s", dec)
	}

	// Hallucination detection test
	fileContent := `package main
import "fmt"
func Run() {
    fmt.Println("hello")
}`

	// Suggestion references non-existent fictitious packages and variables
	sug := domain.CodeSuggestion{
		ID:                   uuid.New(),
		FilePath:             "main.go",
		StartLine:            3,
		EndLine:              4,
		SuggestedReplacement: `quantumCryptor.EncryptWithHyperKey(fictitiousVault, alphaOmegaSecretToken)`,
	}

	isHallucinated, unknowns := eng.DetectHallucinatedIdentifiers(sug, fileContent)
	if !isHallucinated {
		t.Fatal("expected hallucination detector to catch non-existent symbols")
	}
	if len(unknowns) < 3 {
		t.Fatalf("expected at least 3 unknown symbols, got %v", unknowns)
	}
}

func TestCommentAnalysisEngine_ClassificationAndAutoReply(t *testing.T) {
	eng := NewCommentAnalysisEngine()

	targetSug := &domain.CodeSuggestion{
		ID:                   uuid.New(),
		Explanation:          "Response body must be closed to avoid leaking socket connections.",
		SuggestedReplacement: "defer resp.Body.Close()",
	}

	// Agreement test
	res1 := eng.ClassifyDeveloperComment("c1", 10, "alice", "good catch! will fix in next commit", targetSug)
	if res1.Intent != IntentAgreement || !res1.ShouldResolve {
		t.Fatalf("expected agreement and shouldResolve=true, got %+v", res1)
	}

	// Disagreement test
	res2 := eng.ClassifyDeveloperComment("c2", 10, "bob", "this is intentional by design because the pool manages it", targetSug)
	if res2.Intent != IntentDisagreement || !res2.ShouldResolve {
		t.Fatalf("expected disagreement and shouldResolve=true, got %+v", res2)
	}

	// False positive test
	res3 := eng.ClassifyDeveloperComment("c3", 10, "carol", "this is a false positive, the mutex is already held by the caller", targetSug)
	if res3.Intent != IntentFalsePositiveReport || !res3.ShouldMuteRule {
		t.Fatalf("expected false positive report and shouldMuteRule=true, got %+v", res3)
	}

	// Question test
	res4 := eng.ClassifyDeveloperComment("c4", 10, "david", "why do we need defer here? how should this be structured?", targetSug)
	if res4.Intent != IntentQuestion || !strings.Contains(res4.AutomatedReply, "defer resp.Body.Close()") {
		t.Fatalf("expected question intent and contextual auto-reply, got %+v", res4)
	}

	// Rule extraction test
	res5 := eng.ClassifyDeveloperComment("c5", 10, "eve", "in our codebase we always wrap external API calls with circuit breakers", targetSug)
	rules := eng.ExtractRuleCandidates([]DeveloperCommentAnalysis{res5})
	if len(rules) != 1 || !strings.Contains(rules[0].Description, "always wrap external API calls") {
		t.Fatalf("expected 1 extracted rule candidate, got %+v", rules)
	}
}

func TestDeepCommentManager_MarkdownAndLifecycle(t *testing.T) {
	ctx := context.Background()
	mgr := NewDeepCommentManager(nil)
	mockAdapter := NewMockSCMPlatformAdapter()
	mgr.RegisterAdapter("github", mockAdapter)

	reviewID := uuid.New()
	headSHA := "abc1234567890def"
	baseSHA := "fed0987654321cba"

	files := []ChangedFileInfo{
		{Path: "pkg/auth/token.go", Status: "modified", Additions: 45, Deletions: 12, Changes: 57},
		{Path: "pkg/db/query.go", Status: "modified", Additions: 120, Deletions: 30, Changes: 150},
		{Path: "pkg/util/helper.go", Status: "added", Additions: 30, Deletions: 0, Changes: 30},
	}

	suggestions := []domain.CodeSuggestion{
		{
			ID:                   uuid.New(),
			FilePath:             "pkg/auth/token.go",
			StartLine:            15,
			EndLine:              20,
			Severity:             domain.SeverityCritical,
			Category:             domain.CategorySecurity,
			Description:          "Insecure JWT signing key used in production path.",
			SuggestedReplacement: "token.SignWithHS256(secretKey)",
		},
		{
			ID:                   uuid.New(),
			FilePath:             "pkg/db/query.go",
			StartLine:            40,
			EndLine:              45,
			Severity:             domain.SeverityMajor,
			Category:             domain.CategoryBug,
			Description:          "Unchecked database connection error.",
			SuggestedReplacement: "if err != nil { return nil, err }",
		},
		{
			ID:                   uuid.New(),
			FilePath:             "pkg/util/helper.go",
			StartLine:            5,
			EndLine:              8,
			Severity:             domain.SeverityMinor,
			Category:             domain.CategoryPerformance,
			Description:          "Preallocate slice capacity for loop append.",
			SuggestedReplacement: "res := make([]string, 0, len(items))",
		},
	}

	cfg := domain.DefaultCodeReviewConfig()

	// Test 1: GeneratePullRequestSummaryMarkdown
	summaryParams := PRSummaryParams{
		ReviewID:       reviewID,
		Title:          "Refactor Authentication and Database Services",
		PullNumber:     101,
		Author:         "developer-alice",
		HeadSHA:        headSHA,
		BaseSHA:        baseSHA,
		ChangedFiles:   files,
		Suggestions:    suggestions,
		Config:         cfg,
		StartTime:      time.Now().Add(-2 * time.Second),
		Duration:       2 * time.Second,
		AttestationURL: "https://scandrix.dev/attestations/slsa/101",
	}

	markdown, err := mgr.GeneratePullRequestSummaryMarkdown(ctx, summaryParams)
	if err != nil {
		t.Fatalf("failed generating PR summary markdown: %v", err)
	}

	if !strings.Contains(markdown, "Refactor Authentication and Database Services") {
		t.Fatal("expected summary markdown to contain PR title")
	}
	if !strings.Contains(markdown, "Insecure JWT signing key") {
		t.Fatal("expected summary markdown to contain priority action item")
	}
	if !strings.Contains(markdown, "SLSA Provenance Attestation") {
		t.Fatal("expected summary markdown to include SLSA attestation link")
	}

	// Test 2: CalculateRiskScore
	score, level := mgr.CalculateRiskScore(suggestions)
	if score < 50 || level != "HIGH" && level != "CRITICAL" {
		t.Fatalf("expected high/critical risk score for critical security finding, got %d (%s)", score, level)
	}

	// Test 3: ChunkChangedFilesForSummary
	chunks := mgr.ChunkChangedFilesForSummary(files, 2)
	if len(chunks) != 2 || len(chunks[0]) != 2 || len(chunks[1]) != 1 {
		t.Fatalf("expected 2 chunks (2, 1), got %d chunks", len(chunks))
	}

	// Test 4: FormatFileImpactTable
	table := mgr.FormatFileImpactTable(files, suggestions)
	if !strings.Contains(table, "pkg/auth/token.go") || !strings.Contains(table, "+45 / -12") {
		t.Fatalf("expected impact table to contain auth token metrics, got: %s", table)
	}

	// Test 5: Initial Comment Lifecycle
	initialParams := InitialCommentParams{
		ReviewID:          reviewID,
		PullNumber:        101,
		Author:            "developer-alice",
		HeadSHA:           headSHA,
		TotalFiles:        len(files),
		EstimatedDuration: 5 * time.Second,
	}

	initialCommentID, err := mgr.CreateInitialComment(ctx, "github", "acme/repo", 101, initialParams)
	if err != nil {
		t.Fatalf("failed creating initial comment: %v", err)
	}
	if initialCommentID == "" {
		t.Fatal("expected non-empty initial comment ID")
	}

	progressUpdate := ReviewProgressUpdate{
		CurrentStage: "Multi-Agent Deliberation",
		StageIndex:   8,
		TotalStages:  16,
		Percent:      0.50,
		FilesDone:    2,
		TotalFiles:   3,
		Message:      "Running security and performance specialists",
	}

	if err := mgr.UpdateInitialCommentProgress(ctx, "github", "acme/repo", 101, initialCommentID, progressUpdate); err != nil {
		t.Fatalf("failed updating initial comment progress: %v", err)
	}

	if err := mgr.FinishInitialComment(ctx, "github", "acme/repo", 101, initialCommentID, "CHANGES_REQUESTED", 2*time.Second); err != nil {
		t.Fatalf("failed finishing initial comment: %v", err)
	}

	// Test 6: ProcessEndReviewMessageTemplate
	tmpl := "Review {review_id} for PR #{pull_number} by @{author} completed in {elapsed_time} with {suggestions_count} findings ({critical_count} critical)."
	vars := mgr.BuildDefaultTemplateContext(reviewID, "acme/repo", 101, "developer-alice", time.Now().Add(-1*time.Second), suggestions)
	rendered := mgr.ProcessEndReviewMessageTemplate(tmpl, vars)

	if !strings.Contains(rendered, "by @developer-alice") || !strings.Contains(rendered, "3 findings (1 critical)") {
		t.Fatalf("unexpected template render output: %s", rendered)
	}

	// Test 7: SanitizeBitbucketMarkdown
	bbInput := "<details><summary>Details</summary>Inner code\nNext line</details>\n```suggestion\nnewCode()\n```"
	bbOutput := mgr.SanitizeBitbucketMarkdown(bbInput)
	if strings.Contains(bbOutput, "<details>") || !strings.Contains(bbOutput, "#### Details") || !strings.Contains(bbOutput, "```diff") {
		t.Fatalf("failed sanitizing bitbucket markdown: %s", bbOutput)
	}
}

func TestDeepCommentManager_LineCommentsAndRetry(t *testing.T) {
	ctx := context.Background()
	mgr := NewDeepCommentManager(nil)
	mockAdapter := NewMockSCMPlatformAdapter()
	mgr.RegisterAdapter("github", mockAdapter)

	repo := "acme/backend"
	pull := 55
	headSHA := "c0ffee123456"

	comments := []SCMReviewComment{
		{
			ID:           "com-10",
			SuggestionID: uuid.New().String(),
			RuleID:       "security-sql-inject",
			Anchor: SCMCommentAnchor{
				FilePath:       "pkg/api/handler.go",
				Line:           25,
				StartLine:      20,
				Side:           "RIGHT",
				OriginalCommit: headSHA,
			},
			Body:     "Avoid raw sql execution",
			Severity: "CRITICAL",
			Category: "SECURITY",
		},
		{
			ID:           "com-11",
			SuggestionID: uuid.New().String(),
			RuleID:       "style-naming",
			Anchor: SCMCommentAnchor{
				FilePath:       "pkg/api/handler.go",
				Line:           80,
				StartLine:      80,
				Side:           "RIGHT",
				OriginalCommit: headSHA,
			},
			Body:     "Name variable descriptively",
			Severity: "INFO",
			Category: "STYLE",
		},
	}

	// Test 1: CreateLineComments
	ids, errs := mgr.CreateLineComments(ctx, "github", repo, pull, headSHA, comments)
	if len(errs) > 0 {
		t.Fatalf("expected zero errors creating line comments, got %v", errs)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 created comment IDs, got %d", len(ids))
	}

	// Test 2: RepeatedCodeReviewSuggestionClustering
	repeatedSugs := []domain.CodeSuggestion{
		{
			ID:                   uuid.New(),
			RuleID:               "err-handle",
			FilePath:             "a.go",
			StartLine:            10,
			Category:             domain.CategoryBug,
			Severity:             domain.SeverityMajor,
			SuggestedReplacement: "if err != nil { return err }",
		},
		{
			ID:                   uuid.New(),
			RuleID:               "err-handle",
			FilePath:             "a.go",
			StartLine:            50,
			Category:             domain.CategoryBug,
			Severity:             domain.SeverityCritical, // highest severity -> becomes parent
			SuggestedReplacement: "if err != nil { return err }",
		},
		{
			ID:                   uuid.New(),
			RuleID:               "err-handle",
			FilePath:             "b.go",
			StartLine:            15,
			Category:             domain.CategoryBug,
			Severity:             domain.SeverityMinor,
			SuggestedReplacement: "if err != nil { return err }",
		},
	}

	parents, clusters := mgr.RepeatedCodeReviewSuggestionClustering(ctx, repeatedSugs, 5)
	if len(parents) != 1 {
		t.Fatalf("expected 1 parent suggestion from repeated clustering, got %d", len(parents))
	}
	if parents[0].Severity != domain.SeverityCritical {
		t.Fatalf("expected parent to be promoted to highest severity (CRITICAL), got %s", parents[0].Severity)
	}

	enrichedParents := mgr.EnrichParentSuggestionsWithRelated(parents, clusters)
	if !strings.Contains(enrichedParents[0].GetExplanation(), "Also detected at 2 other locations") {
		t.Fatalf("expected parent explanation to list other occurrences, got: %s", enrichedParents[0].GetExplanation())
	}

	clusteredIDs := mgr.ExtractAllClusteredIDs(clusters)
	if len(clusteredIDs) != 3 {
		t.Fatalf("expected 3 clustered IDs, got %d", len(clusteredIDs))
	}

	idMap := make(map[string]bool)
	for _, id := range clusteredIDs {
		idMap[id] = true
	}
	nonClustered := mgr.FilterNonClusteredSuggestions(repeatedSugs, idMap)
	if len(nonClustered) != 0 {
		t.Fatalf("expected 0 non-clustered suggestions, got %d", len(nonClustered))
	}

	// Test 3: PR-Level Comments
	prComments := []SCMReviewComment{
		{
			ID:       "pr-com-1",
			Category: "ARCHITECTURE",
			Body:     "Consider splitting this module into separate packages.",
		},
	}
	prIDs, err := mgr.CreatePrLevelReviewComments(ctx, "github", repo, pull, prComments)
	if err != nil || len(prIDs) != 1 {
		t.Fatalf("failed creating PR level review comment: %v", err)
	}
}

func TestDeepSuggestionService_Extended(t *testing.T) {
	ctx := context.Background()
	svc := NewDeepSuggestionService(nil, nil)
	mockAdapter := NewMockSCMPlatformAdapter()

	sug1 := domain.CodeSuggestion{
		ID:                   uuid.New(),
		FilePath:             "pkg/auth/login.go",
		StartLine:            20,
		EndLine:              25,
		Severity:             domain.SeverityCritical,
		Category:             domain.CategorySecurity,
		SuggestedReplacement: "bcrypt.CompareHashAndPassword(hashed, password)",
		OriginalDiff:         "password == storedPassword",
		Comment: &domain.SCMCommentRef{
			PlatformCommentID: "th-login-auth",
		},
	}

	sug2 := domain.CodeSuggestion{
		ID:                   uuid.New(),
		FilePath:             "pkg/auth/login.go",
		StartLine:            40,
		EndLine:              45,
		Severity:             domain.SeverityMinor,
		Category:             domain.CategoryStyle,
		SuggestedReplacement: "return fmt.Errorf(\"invalid user\")",
		OriginalDiff:         "return errors.New(\"invalid user\")",
		Comment: &domain.SCMCommentRef{
			PlatformCommentID: "th-style-err",
		},
	}

	// Test 1: ValidateImplementedSuggestions
	// Simulated PR head commit where developer adopted bcrypt!
	newContents := map[string]string{
		"pkg/auth/login.go": "package auth\nfunc Login() {\n    bcrypt.CompareHashAndPassword(hashed, password)\n    return errors.New(\"invalid user\")\n}",
	}

	implemented, pending := svc.ValidateImplementedSuggestions(ctx, "acme/repo", 1, []domain.CodeSuggestion{sug1, sug2}, newContents)
	if len(implemented) != 1 || implemented[0].ID != sug1.ID {
		t.Fatalf("expected sug1 to be recognized as implemented, got %d implemented", len(implemented))
	}
	if len(pending) != 1 || pending[0].ID != sug2.ID {
		t.Fatalf("expected sug2 to remain pending, got %d pending", len(pending))
	}

	// Test 2: ResolveImplementedSuggestionsOnPlatform
	resolvedCount, err := svc.ResolveImplementedSuggestionsOnPlatform(ctx, mockAdapter, "acme/repo", 1, implemented)
	if err != nil || resolvedCount != 1 {
		t.Fatalf("expected 1 resolved suggestion on platform, got count=%d, err=%v", resolvedCount, err)
	}
	if mockAdapter.Resolved["th-login-auth"] != ThreadStatusFixed {
		t.Fatalf("expected th-login-auth to be marked FIXED, got %s", mockAdapter.Resolved["th-login-auth"])
	}

	// Test 3: PrioritizeSuggestionsWithDrixyRulesControl
	ruleSug := domain.CodeSuggestion{
		ID:       uuid.New(),
		RuleID:   "enterprise-rule-sec-01",
		Severity: domain.SeverityMajor,
	}
	generalSug := domain.CodeSuggestion{
		ID:       uuid.New(),
		RuleID:   "",
		Severity: domain.SeverityCritical,
	}

	limits := SeverityLimitsConfig{MaxTotal: 1}
	prio, disc := svc.PrioritizeSuggestionsWithDrixyRulesControl([]domain.CodeSuggestion{generalSug, ruleSug}, []string{"enterprise-rule-sec-01"}, limits)
	if len(prio) != 1 || prio[0].ID != ruleSug.ID {
		t.Fatalf("expected enterprise rule suggestion to take top priority slot, got %v", prio)
	}
	if len(disc) != 1 || disc[0].ID != generalSug.ID {
		t.Fatalf("expected general suggestion to be discarded due to budget, got %v", disc)
	}

	// Test 4: PrioritizeSuggestionsByFile
	fileSugs := []domain.CodeSuggestion{
		{ID: uuid.New(), FilePath: "a.go", Severity: domain.SeverityCritical, PriorityScore: 90},
		{ID: uuid.New(), FilePath: "a.go", Severity: domain.SeverityMajor, PriorityScore: 80},
		{ID: uuid.New(), FilePath: "a.go", Severity: domain.SeverityMinor, PriorityScore: 40},
	}
	keptFile, discFile := svc.PrioritizeSuggestionsByFile(fileSugs, 2)
	if len(keptFile) != 2 || len(discFile) != 1 {
		t.Fatalf("expected 2 kept and 1 discarded for file a.go, got kept=%d, disc=%d", len(keptFile), len(discFile))
	}

	// Test 5: PrioritizeSuggestionsByPR
	prSugs := []domain.CodeSuggestion{
		{ID: uuid.New(), FilePath: "file1.go", Severity: domain.SeverityCritical},
		{ID: uuid.New(), FilePath: "file1.go", Severity: domain.SeverityMajor},
		{ID: uuid.New(), FilePath: "file2.go", Severity: domain.SeverityMajor},
		{ID: uuid.New(), FilePath: "file3.go", Severity: domain.SeverityMinor},
	}
	keptPR, discPR := svc.PrioritizeSuggestionsByPR(prSugs, 2)
	if len(keptPR) != 2 || len(discPR) != 2 {
		t.Fatalf("expected 2 kept and 2 discarded for PR, got kept=%d, disc=%d", len(keptPR), len(discPR))
	}

	// Test 6: NormalizeSeverity
	normMinor := svc.NormalizeSeverity(domain.CodeSuggestion{Severity: domain.SeverityMinor}, true, 0)
	if normMinor != domain.SeverityMajor {
		t.Fatalf("expected minor severity on security sensitive path to promote to MAJOR, got %s", normMinor)
	}
	normCaller := svc.NormalizeSeverity(domain.CodeSuggestion{Severity: domain.SeverityMajor}, false, 25)
	if normCaller != domain.SeverityCritical {
		t.Fatalf("expected major severity with >20 callers to promote to CRITICAL, got %s", normCaller)
	}

	// Test 7: CalculateSuggestionRankScoreWithContext
	rankScore := svc.CalculateSuggestionRankScoreWithContext(domain.CodeSuggestion{Severity: domain.SeverityCritical, Confidence: 0.9}, 0.8, 0.7)
	if rankScore <= 60.0 {
		t.Fatalf("expected high rank score for critical finding, got %f", rankScore)
	}

	// Test 8: FilterSuggestionsBySeverityLevel and ProcessSeverityFilter
	mixedSugs := []domain.CodeSuggestion{
		{ID: uuid.New(), Severity: domain.SeverityCritical},
		{ID: uuid.New(), Severity: domain.SeverityMajor},
		{ID: uuid.New(), Severity: domain.SeverityMinor},
		{ID: uuid.New(), Severity: domain.SeverityInfo},
	}

	atLeastMajor := svc.FilterSuggestionsBySeverityLevel(mixedSugs, domain.SeverityMajor)
	if len(atLeastMajor) != 2 {
		t.Fatalf("expected 2 suggestions at least MAJOR, got %d", len(atLeastMajor))
	}

	exactSev := svc.ProcessSeverityFilter(mixedSugs, []domain.ReviewSeverity{domain.SeverityCritical, domain.SeverityInfo})
	if len(exactSev) != 2 {
		t.Fatalf("expected 2 suggestions matching CRITICAL and INFO, got %d", len(exactSev))
	}

	tally := svc.AnalyzeSuggestionsSeverity(mixedSugs)
	if tally[domain.SeverityCritical] != 1 || tally[domain.SeverityMajor] != 1 {
		t.Fatalf("unexpected severity breakdown: %v", tally)
	}
}

func TestSafeguardPipelineEngine_MultiLanguageAndStaticChecks(t *testing.T) {
	eng := NewSafeguardPipelineEngine(nil)

	// Test 1: TypeScript Symbol Parsing
	tsCode := `
export async function authenticateUser(token: string): Promise<User> { return null; }
export class AuthenticationService {
    private secret: string;
}
export interface UserPayload {
    id: string;
}
export type TokenResponse = { token: string };
export const DEFAULT_TIMEOUT = 5000;
`
	tsSymbols, err := ParseTypeScriptSymbols(tsCode)
	if err != nil || len(tsSymbols) != 5 {
		t.Fatalf("expected 5 TypeScript symbols, got %d (err=%v)", len(tsSymbols), err)
	}

	// Test 2: Python Symbol Parsing
	pyCode := `
async def fetch_user_data(user_id):
    pass

class DataPipeline:
    pass

DEFAULT_RETRIES = 3
`
	pySymbols, err := ParsePythonSymbols(pyCode)
	if err != nil || len(pySymbols) != 3 {
		t.Fatalf("expected 3 Python symbols, got %d (err=%v)", len(pySymbols), err)
	}

	// Test 3: Java Symbol Parsing
	javaCode := `
public class PaymentGateway {
    public void processPayment(String card) {
    }
}
public interface Tokenizer {
}
`
	javaSymbols, err := ParseJavaSymbols(javaCode)
	if err != nil || len(javaSymbols) != 3 {
		t.Fatalf("expected 3 Java symbols, got %d (err=%v)", len(javaSymbols), err)
	}

	// Test 4: Rust Symbol Parsing
	rustCode := `
pub async fn verify_proof(hash: &[u8]) -> bool { true }
pub struct MerkleTree {
    root: Vec<u8>,
}
pub enum ProofType {
    Inclusion,
}
pub trait Verifiable {
}
`
	rustSymbols, err := ParseRustSymbols(rustCode)
	if err != nil || len(rustSymbols) != 4 {
		t.Fatalf("expected 4 Rust symbols, got %d (err=%v)", len(rustSymbols), err)
	}

	// Test 5: C# Symbol Parsing
	csCode := `
public class OrderService {
    public async Task<Order> CreateOrder(OrderDto dto) {
    }
}
public record OrderDto(int Id);
public interface IOrderService {
}
`
	csSymbols, err := ParseCSharpSymbols(csCode)
	if err != nil || len(csSymbols) != 4 {
		t.Fatalf("expected 4 C# symbols, got %d (err=%v)", len(csSymbols), err)
	}

	// Test 6: VerifyContractConsistency
	existFunc := "func HandleRequest(w http.ResponseWriter, r *http.Request)"
	renamedFunc := "func HandleApiRequest(w http.ResponseWriter, r *http.Request)"
	driftParamsFunc := "func HandleRequest(w http.ResponseWriter, r *http.Request, ctx context.Context)"

	ok1, _ := eng.VerifyContractConsistency(existFunc, renamedFunc, "go")
	if ok1 {
		t.Fatal("expected contract check to fail for renamed function")
	}

	ok2, _ := eng.VerifyContractConsistency(existFunc, driftParamsFunc, "go")
	if ok2 {
		t.Fatal("expected contract check to fail for parameter count drift")
	}

	// Test 7: DetectResourceLeaks
	leakCodeGo := `
f, err := os.Open("data.txt")
data := readAll(f)
`
	isLeak, _ := eng.DetectResourceLeaks(leakCodeGo, "go")
	if !isLeak {
		t.Fatal("expected resource leak detector to catch unclosed file handle in Go")
	}

	noLeakCodeGo := `
f, err := os.Open("data.txt")
defer f.Close()
`
	isNoLeak, _ := eng.DetectResourceLeaks(noLeakCodeGo, "go")
	if isNoLeak {
		t.Fatal("expected no resource leak when defer f.Close() is present")
	}

	// Test 8: VerifyErrorHandling
	suppressedErrCode := `
res, err := compute()
_ = err
`
	errOk, _ := eng.VerifyErrorHandling(suppressedErrCode, "go")
	if errOk {
		t.Fatal("expected error handling check to fail for `_ = err`")
	}

	// Test 9: AnalyzeTaintFlow
	taintCode := `
query := fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", userInput)
db.Query(query)
`
	isTaint, _ := eng.AnalyzeTaintFlow(taintCode, "go")
	if !isTaint {
		t.Fatal("expected taint flow analysis to catch direct string interpolation into SQL query sink")
	}

	// Test 10: VerifyWithPromptOnly
	redundantSug := domain.CodeSuggestion{
		SuggestedReplacement: "return x",
		OriginalDiff:         "return x",
	}
	valid, _ := eng.VerifyWithPromptOnly(context.Background(), redundantSug, "")
	if valid {
		t.Fatal("expected prompt-only verifier to reject suggestion identical to existing code")
	}

	todoSug := domain.CodeSuggestion{
		SuggestedReplacement: "// TODO: implement logic later",
		OriginalDiff:         "return nil",
		StartLine:            10,
		EndLine:              15,
	}
	validTodo, _ := eng.VerifyWithPromptOnly(context.Background(), todoSug, "")
	if validTodo {
		t.Fatal("expected prompt-only verifier to reject suggestion with TODO comment")
	}
}

func TestCommentAnalysisEngine_BotAndDenylistFilters(t *testing.T) {
	eng := NewCommentAnalysisEngine()

	// Test 1: Bot Filter
	botFilter := NewBotFilter()
	if !botFilter.IsBotComment("dependabot[bot]", "Bumps lodash from 4.17.20 to 4.17.21") {
		t.Fatal("expected dependabot[bot] to be recognized as bot")
	}
	if !botFilter.IsBotComment("github-actions[bot]", "Tests failed on Node 20") {
		t.Fatal("expected github-actions[bot] to be recognized as bot")
	}
	if !botFilter.IsBotComment("developer-bob", "<!-- codecov --> Coverage increased by 0.5%") {
		t.Fatal("expected signature match on codecov comment body")
	}
	if botFilter.IsBotComment("alice-engineer", "LGTM, looks clean!") {
		t.Fatal("did not expect human engineer to be marked as bot")
	}

	// Test 2: Reviewer Filter
	reviewerFilter := NewReviewerFilter([]string{"bot-user", "qa-automation"}, nil)
	if !reviewerFilter.IsExcluded("bot-user") {
		t.Fatal("expected bot-user to be excluded by denylist")
	}
	if reviewerFilter.IsExcluded("lead-dev") {
		t.Fatal("expected lead-dev to be allowed")
	}

	allowlistFilter := NewReviewerFilter(nil, []string{"lead-dev", "architect"})
	if allowlistFilter.IsExcluded("lead-dev") {
		t.Fatal("expected lead-dev to be in allowlist")
	}
	if !allowlistFilter.IsExcluded("random-contributor") {
		t.Fatal("expected random-contributor to be excluded when allowlist is active")
	}

	// Test 3: Batch CategorizeComments
	comments := []SCMReviewComment{
		{
			ID:     "c1",
			Body:   "dependabot: automatic dependency update",
			Anchor: SCMCommentAnchor{OriginalCommit: "dependabot[bot]"},
		},
		{
			ID:     "c2",
			Body:   "in this project we always write unit tests for all domain services",
			Anchor: SCMCommentAnchor{OriginalCommit: "senior-architect"},
		},
		{
			ID:     "c3",
			Body:   "looks good to me, great refactoring!",
			Anchor: SCMCommentAnchor{OriginalCommit: "peer-reviewer"},
		},
	}

	params := CommentCategorizationParams{
		Comments: comments,
	}

	analyses, synthesizedRules := eng.CategorizeComments(params)
	// c1 should be skipped because it's a bot!
	if len(analyses) != 2 {
		t.Fatalf("expected 2 analyzed comments (excluding bot), got %d", len(analyses))
	}
	if len(synthesizedRules) != 1 {
		t.Fatalf("expected 1 synthesized rule candidate, got %d", len(synthesizedRules))
	}
	if !strings.Contains(synthesizedRules[0].Description, "always write unit tests") {
		t.Fatalf("unexpected synthesized rule: %+v", synthesizedRules[0])
	}
}

