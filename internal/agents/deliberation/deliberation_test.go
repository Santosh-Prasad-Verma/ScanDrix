package deliberation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agents/deliberation"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

func TestAgentDeliberationAndConsensus(t *testing.T) {
	ctx := context.Background()
	deliberator := deliberation.NewAgentDeliberator(0.70)

	// 1. True Positive: Security flaw supported by consensus
	cand1 := deliberation.ProposeFinding(
		deliberation.PersonaSecurityAuditor,
		"internal/db/query.go",
		45, 48,
		"SQL Injection via Unsanitized Param",
		models.SeverityCritical,
		0.95,
		"Direct string concatenation into DB query execution",
		"Use db.QueryContext(ctx, query, arg1, arg2)",
	)

	// Clean Code Reviewer agrees with Security Auditor
	crit1 := deliberation.ProposeCritique(
		cand1.ID,
		deliberation.PersonaCleanCodeReviewer,
		deliberation.VerdictAgree,
		"Verified. Raw concatenation violates repository repository guidelines.",
		0.0,
	)

	findings1, decisions1 := deliberator.DeliberateExec(ctx, []deliberation.CandidateFinding{cand1}, []deliberation.PeerCritique{crit1})
	if len(findings1) != 1 || len(decisions1) != 1 {
		t.Fatalf("expected 1 accepted finding, got %d findings, %d decisions", len(findings1), len(decisions1))
	}
	if !decisions1[0].ConsensusReached || decisions1[0].IsFilteredOut || decisions1[0].FinalConfidence < 0.90 {
		t.Fatalf("expected robust consensus on true positive: %+v", decisions1[0])
	}

	// 2. False Positive: Hallucinated nitpick struck down by Devil's Advocate
	cand2 := deliberation.ProposeFinding(
		deliberation.PersonaCleanCodeReviewer,
		"internal/api/handler.go",
		120, 122,
		"Potential Panic on Missing Header",
		models.SeverityMedium,
		0.72,
		"Header value accessed without nil check",
		"Check if header exists before reading",
	)

	// Devil's Advocate proves the header is guaranteed by earlier AuthMiddleware
	crit2 := deliberation.ProposeCritique(
		cand2.ID,
		deliberation.PersonaDevilsAdvocate,
		deliberation.VerdictFalsePositive,
		"False Positive: AuthMiddleware executes prior to this handler and guarantees header presence.",
		0.45, // Heavy penalty
	)

	findings2, decisions2 := deliberator.DeliberateExec(ctx, []deliberation.CandidateFinding{cand2}, []deliberation.PeerCritique{crit2})
	if len(findings2) != 0 {
		t.Fatalf("expected false positive to be completely filtered out, got %d findings", len(findings2))
	}
	if len(decisions2) != 1 || !decisions2[0].IsFilteredOut || decisions2[0].ConsensusReached {
		t.Fatalf("expected decision marked filtered out: %+v", decisions2[0])
	}
	if decisions2[0].FinalConfidence >= 0.70 {
		t.Fatalf("expected confidence to drop below threshold, got: %f", decisions2[0].FinalConfidence)
	}

	// 3. Overlapping Multi-Persona Merge
	cand3A := deliberation.ProposeFinding(
		deliberation.PersonaSecurityAuditor,
		"internal/cache/store.go",
		30, 32,
		"Unbounded Memory Cache Allocation",
		models.SeverityHigh,
		0.85,
		"Map grows indefinitely without eviction policy",
		"Use LRU cache with maximum capacity",
	)
	cand3B := deliberation.ProposeFinding(
		deliberation.PersonaPerformanceArchitect,
		"internal/cache/store.go",
		31, 33,
		"Memory Pressure from Cache Misses",
		models.SeverityHigh,
		0.88,
		"Goroutine cache population causes high heap allocations",
		"Implement singleflight to coalesce cache misses",
	)

	findings3, decisions3 := deliberator.DeliberateExec(ctx, []deliberation.CandidateFinding{cand3A, cand3B}, nil)
	if len(findings3) != 1 || len(decisions3) != 1 {
		t.Fatalf("expected 2 overlapping candidates to merge into 1 finding, got %d findings", len(findings3))
	}
	if len(decisions3[0].SupportingPersonas) != 2 {
		t.Fatalf("expected 2 supporting personas, got %d", len(decisions3[0].SupportingPersonas))
	}
}

func TestSpecializedPersonasAndMultiTurnDeliberation(t *testing.T) {
	ctx := context.Background()
	deliberator := deliberation.NewAgentDeliberator(0.70)

	// Sample diff containing:
	// 1. Concurrency map race
	// 2. Unclosed HTTP response body (Memory leak)
	// 3. N+1 SQL query inside loop
	sampleDiff := `diff --git a/worker/concurrency.go b/worker/concurrency.go
--- a/worker/concurrency.go
+++ b/worker/concurrency.go
@@ -1,10 +1,15 @@
 package worker
+func runConcurrent() {
+	cache := make(map[int]string)
+	go func() {
+		cache[1] = "bad"
+	}()
+}
diff --git a/worker/memory.go b/worker/memory.go
--- a/worker/memory.go
+++ b/worker/memory.go
@@ -1,10 +1,15 @@
 package worker
+func fetchMetrics() {
+	resp, err := http.Get("http://api.internal")
+}
diff --git a/worker/database.go b/worker/database.go
--- a/worker/database.go
+++ b/worker/database.go
@@ -1,10 +1,15 @@
 package worker
+func loadUsers(db *sql.DB, ids []int) {
+	for _, id := range ids {
+		rows, err := db.Query("SELECT * FROM users WHERE id = ?", id)
+	}
+}
+`

	patches, err := diff.ParseUnifiedDiff(strings.NewReader(sampleDiff))
	if err != nil {
		t.Fatalf("failed parsing diff: %v", err)
	}

	state, findings := deliberator.DeliberateMultiTurn(ctx, patches, nil)

	if state == nil || len(state.Rounds) != 3 {
		t.Fatalf("expected 3 deliberation rounds, got: %+v", state)
	}

	if len(findings) < 2 {
		t.Fatalf("expected at least 2 specialized findings, got: %d", len(findings))
	}

	// Verify detection of specialized personas
	hasConcurrency := false
	hasMemoryLeak := false
	hasSQL := false

	for _, f := range findings {
		if f.Category == string(deliberation.PersonaConcurrencyAuditor) {
			hasConcurrency = true
		}
		if f.Category == string(deliberation.PersonaMemoryLeakSpecialist) {
			hasMemoryLeak = true
		}
		if f.Category == string(deliberation.PersonaSQLOptimizer) {
			hasSQL = true
		}
	}

	if !hasConcurrency && !hasMemoryLeak && !hasSQL {
		t.Fatalf("expected at least one specialized category detection in findings: %+v", findings)
	}

	// Verify default personas catalog size
	allPersonas := deliberation.GetDefaultPersonas()
	if len(allPersonas) != 7 {
		t.Fatalf("expected 7 specialized personas, got: %d", len(allPersonas))
	}
}
