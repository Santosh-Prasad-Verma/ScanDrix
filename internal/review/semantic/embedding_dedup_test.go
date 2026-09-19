// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package semantic

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

type mockEmbedder struct {
	embeddings map[string][]float32
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if emb, ok := m.embeddings[t]; ok {
			out[i] = emb
		} else {
			// Deterministic pseudo-embedding based on hash
			out[i] = generateDeterministicEmbedding(t, 8)
		}
	}
	return out, nil
}

func generateDeterministicEmbedding(text string, dim int) []float32 {
	vec := make([]float32, dim)
	for i, c := range text {
		vec[i%dim] += float32(c)
	}
	// Normalize
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] /= float32(norm)
		}
	}
	return vec
}

func TestTokenizeWords(t *testing.T) {
	text := "Check nil pointer dereference on user.ID! Also SQL_Injection vulnerability."
	tokens := TokenizeWords(text)

	expectedTokens := []string{"check", "nil", "pointer", "dereference", "user", "also", "sql_injection", "vulnerability"}
	for _, exp := range expectedTokens {
		if _, ok := tokens[exp]; !ok {
			t.Errorf("expected token '%s' to be present", exp)
		}
	}

	// Two-letter word 'on' or 'id' should be omitted (< 3 chars)
	if _, ok := tokens["on"]; ok {
		t.Errorf("token 'on' (< 3 chars) should be omitted")
	}
	if _, ok := tokens["id"]; ok {
		t.Errorf("token 'id' (< 3 chars) should be omitted")
	}
}

func TestJaccardSimilarity(t *testing.T) {
	setA := map[string]struct{}{"apple": {}, "banana": {}, "orange": {}}
	setB := map[string]struct{}{"banana": {}, "orange": {}, "grape": {}}

	// Intersection: {banana, orange} (2)
	// Union: {apple, banana, orange, grape} (4)
	// Jaccard: 2 / 4 = 0.5
	sim := JaccardSimilarity(setA, setB)
	if math.Abs(sim-0.5) > 1e-4 {
		t.Errorf("expected Jaccard similarity 0.5, got %f", sim)
	}

	// Empty sets
	if JaccardSimilarity(nil, setA) != 0.0 {
		t.Errorf("expected 0.0 for nil set")
	}
}

func TestCalculateContentSimilarity_WithDiff(t *testing.T) {
	findingA := &models.CodeFinding{
		Title:         "Unhandled error on file close",
		Description:   "Defer close without checking the error return value.",
		Remediation:   "Check err returned by f.Close()",
		SuggestedDiff: "- defer f.Close()\n+ defer func() { _ = f.Close() }()",
	}

	findingB := &models.CodeFinding{
		Title:         "Unhandled error closing file descriptor",
		Description:   "Defer close without checking error return.",
		Remediation:   "Capture and log err from f.Close()",
		SuggestedDiff: "- defer f.Close()\n+ defer func() { if err := f.Close(); err != nil {} }()",
	}

	sim := CalculateContentSimilarity(findingA, findingB)
	if sim < 0.3 {
		t.Errorf("expected content similarity >= 0.3, got %f", sim)
	}
}

func TestCollapseNearDuplicates(t *testing.T) {
	// 3 findings in file A: two are duplicates, one has richer detail
	f1 := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/auth/token.go",
		Title:       "Missing nil check",
		Description: "token could be nil",
		Remediation: "Add nil check",
	}
	f2 := &models.CodeFinding{
		ID:            uuid.New(),
		FilePath:      "pkg/auth/token.go",
		Title:         "Missing nil pointer check on token",
		Description:   "token could be nil when user is unauthenticated, leading to SIGSEGV panic.",
		Remediation:   "Add if token == nil validation check before accessing claims.",
		SuggestedDiff: "+ if token == nil { return ErrUnauthorized }",
	}
	// 1 distinct finding in file A
	f3 := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/auth/token.go",
		Title:       "Hardcoded JWT secret key",
		Description: "JWT HMAC secret key is hardcoded in source file.",
		Remediation: "Load secret key from environment variable.",
	}
	// 1 finding in file B
	f4 := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/db/postgres.go",
		Title:       "SQL Injection risk",
		Description: "Raw query concatenation using fmt.Sprintf.",
		Remediation: "Use parameterized query placeholders.",
	}

	candidates := []*models.CodeFinding{f1, f2, f3, f4}
	collapsed := CollapseNearDuplicates(candidates, 0.3)

	if len(collapsed) != 3 {
		t.Fatalf("expected 3 collapsed findings, got %d", len(collapsed))
	}

	// Verify that f2 was kept over f1 (f2 has higher detail score)
	foundF2 := false
	for _, c := range collapsed {
		if c.ID == f2.ID {
			foundF2 = true
		}
		if c.ID == f1.ID {
			t.Errorf("f1 should have been collapsed into f2")
		}
	}
	if !foundF2 {
		t.Errorf("expected f2 (higher detail) to be retained")
	}
}

func TestCosineSimilarity(t *testing.T) {
	vecA := []float32{1.0, 0.0, 0.0}
	vecB := []float32{1.0, 0.0, 0.0}
	vecC := []float32{0.0, 1.0, 0.0}

	// Identical
	if sim := CosineSimilarity(vecA, vecB); math.Abs(sim-1.0) > 1e-4 {
		t.Errorf("expected cosine similarity 1.0 for identical vectors, got %f", sim)
	}

	// Orthogonal
	if sim := CosineSimilarity(vecA, vecC); math.Abs(sim-0.0) > 1e-4 {
		t.Errorf("expected cosine similarity 0.0 for orthogonal vectors, got %f", sim)
	}

	// Empty
	if sim := CosineSimilarity(nil, vecA); sim != 0.0 {
		t.Errorf("expected 0.0 for nil vector")
	}

	// Float64
	vec64A := []float64{0.5, 0.5}
	vec64B := []float64{0.5, 0.5}
	if sim := CosineSimilarityFloat64(vec64A, vec64B); math.Abs(sim-1.0) > 1e-4 {
		t.Errorf("expected 1.0 for float64 vectors, got %f", sim)
	}
}

func TestSemanticDeduplicator_BatchAndCache(t *testing.T) {
	mock := &mockEmbedder{
		embeddings: make(map[string][]float32),
	}

	dedup := NewSemanticDeduplicator(mock, 0.3, 0.4, 0.7)

	repoID := "repo-scandrix-core"
	pullNumber := 101

	// Batch 1: Two findings
	finding1 := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/storage/s3.go",
		StartLine:   40,
		EndLine:     50,
		Title:       "Missing bucket existence check",
		Description: "Bucket may not exist before uploading object.",
		Remediation: "Call HeadBucket to verify.",
		Category:    "Reliability",
	}
	finding2 := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/storage/s3.go",
		StartLine:   80,
		EndLine:     90,
		Title:       "Unencrypted S3 PutObject payload",
		Description: "SSE-KMS header is missing on PutObjectInput.",
		Remediation: "Set ServerSideEncryption to aws.String(types.ServerSideEncryptionAwsKms).",
		Category:    "Security",
	}

	mock.embeddings[ExtractEmbeddingText(finding1)] = []float32{1.0, 0.0, 0.0, 0.0}
	mock.embeddings[ExtractEmbeddingText(finding2)] = []float32{0.0, 1.0, 0.0, 0.0}

	retained, dups, err := dedup.DeduplicateBatch(context.Background(), repoID, pullNumber, []*models.CodeFinding{finding1, finding2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(retained) != 2 {
		t.Fatalf("expected 2 retained findings, got %d", len(retained))
	}
	if len(dups) != 0 {
		t.Fatalf("expected 0 duplicates in initial run, got %d", len(dups))
	}

	// Batch 2: New commit pushes an exact duplicate of finding1
	finding1Dup := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/storage/s3.go",
		StartLine:   40,
		EndLine:     50,
		Title:       "Missing bucket existence check",
		Description: "Bucket may not exist before uploading object.",
		Remediation: "Call HeadBucket to verify.",
		Category:    "Reliability",
	}
	// And a brand new finding
	finding3 := &models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/storage/s3.go",
		StartLine:   120,
		EndLine:     130,
		Title:       "Resource leak in response body",
		Description: "Response body stream is not closed.",
		Remediation: "defer resp.Body.Close()",
		Category:    "BugRisk",
	}
	mock.embeddings[ExtractEmbeddingText(finding1Dup)] = []float32{1.0, 0.0, 0.0, 0.0}
	mock.embeddings[ExtractEmbeddingText(finding3)] = []float32{0.0, 0.0, 1.0, 0.0}

	retained2, dups2, err := dedup.DeduplicateBatch(context.Background(), repoID, pullNumber, []*models.CodeFinding{finding1Dup, finding3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(retained2) != 1 || retained2[0].ID != finding3.ID {
		t.Errorf("expected only finding3 to be retained")
	}
	if len(dups2) != 1 {
		t.Fatalf("expected 1 duplicate detected, got %d", len(dups2))
	}
	if dups2[0].MatchingTier != "EXACT_FINGERPRINT" {
		t.Errorf("expected EXACT_FINGERPRINT match, got %s", dups2[0].MatchingTier)
	}

	// Reset cache
	dedup.ClearPRCache(repoID, pullNumber)

	// Now re-running finding1 should be accepted since cache was cleared
	retained3, _, _ := dedup.DeduplicateBatch(context.Background(), repoID, pullNumber, []*models.CodeFinding{finding1Dup})
	if len(retained3) != 1 {
		t.Errorf("expected finding to be retained after cache clear")
	}
}

func TestSemanticDeduplicator_ConcurrentRace(t *testing.T) {
	dedup := NewSemanticDeduplicator(nil, 0.3, 0.4, 0.7)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			repoID := fmt.Sprintf("repo-%d", workerID%3)
			pullNumber := workerID % 5

			f := &models.CodeFinding{
				ID:          uuid.New(),
				FilePath:    fmt.Sprintf("pkg/file_%d.go", workerID),
				StartLine:   10,
				EndLine:     20,
				Title:       fmt.Sprintf("Concurrent issue %d", workerID),
				Description: "Potential race condition in shared state.",
				Category:    "Concurrency",
			}
			_, _, _ = dedup.DeduplicateBatch(context.Background(), repoID, pullNumber, []*models.CodeFinding{f})
		}(i)
	}
	wg.Wait()
}

func TestBuildTiebreakPrompt(t *testing.T) {
	fA := &models.CodeFinding{
		FilePath:      "server/http.go",
		StartLine:     25,
		EndLine:       30,
		Title:         "Unbounded request body read",
		Description:   "io.ReadAll without http.MaxBytesReader causes OOM denial of service.",
		Remediation:   "Wrap body with http.MaxBytesReader.",
		SuggestedDiff: "- body, _ := io.ReadAll(r.Body)\n+ body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))",
	}
	fB := &models.CodeFinding{
		FilePath:    "server/http.go",
		StartLine:   25,
		EndLine:     30,
		Title:       "Goroutine leak in request handler",
		Description: "Background goroutine spawned without context cancellation check.",
		Remediation: "Pass req.Context() into worker function.",
	}

	prompt := BuildTiebreakPrompt(fA, fB)
	if !strings.Contains(prompt, "Unbounded request body read") {
		t.Errorf("expected prompt to contain finding A title")
	}
	if !strings.Contains(prompt, "Goroutine leak in request handler") {
		t.Errorf("expected prompt to contain finding B title")
	}
	if !strings.Contains(prompt, "sameBug = TRUE") {
		t.Errorf("expected prompt to contain evaluation rubric")
	}
}
