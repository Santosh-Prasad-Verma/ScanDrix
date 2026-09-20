// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package feedback_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm/embedding"
	"github.com/scandrix/backend/internal/review/feedback"
	"github.com/scandrix/backend/pkg/models"
)

// mockSecurityMemoryStore implements feedback.SecurityMemoryStore in memory for tests.
type mockSecurityMemoryStore struct {
	records      map[uuid.UUID]*database.SecurityMemoryRecord
	fingerprints map[string]*database.SecurityMemoryRecord
	vectors      map[uuid.UUID][]float32
}

func newMockSecurityMemoryStore() *mockSecurityMemoryStore {
	return &mockSecurityMemoryStore{
		records:      make(map[uuid.UUID]*database.SecurityMemoryRecord),
		fingerprints: make(map[string]*database.SecurityMemoryRecord),
		vectors:      make(map[uuid.UUID][]float32),
	}
}

func (m *mockSecurityMemoryStore) UpsertSecurityMemoryWithEmbedding(_ context.Context, _ uuid.UUID, mem *database.SecurityMemoryRecord, emb []float32) error {
	m.records[mem.ID] = mem
	if mem.FindingFingerprint != "" {
		m.fingerprints[mem.FindingFingerprint] = mem
	}
	if len(emb) > 0 {
		m.vectors[mem.ID] = emb
	}
	return nil
}

func (m *mockSecurityMemoryStore) SearchSimilarSecurityFindings(_ context.Context, _ uuid.UUID, emb []float32, category string, _ int, maxDistance float64) ([]database.SecurityMemoryRecord, error) {
	var results []database.SecurityMemoryRecord
	for id, storedVec := range m.vectors {
		rec := m.records[id]
		if category != "" && rec.Category != category {
			continue
		}
		dist := embedding.CosineDistance(emb, storedVec)
		if dist <= maxDistance {
			r := *rec
			r.SimilarityScore = 1.0 - dist
			results = append(results, r)
		}
	}
	return results, nil
}

func (m *mockSecurityMemoryStore) GetSecurityMemoryByFingerprint(_ context.Context, _ uuid.UUID, fingerprint string) (*database.SecurityMemoryRecord, error) {
	if rec, ok := m.fingerprints[fingerprint]; ok {
		return rec, nil
	}
	return nil, nil
}

func TestSemanticFeedbackService_ExactFingerprintSuppression(t *testing.T) {
	store := newMockSecurityMemoryStore()
	embedder := embedding.NewDeterministicSemanticEmbedder()
	svc := feedback.NewSemanticFeedbackService(store, embedder)

	wsID := uuid.New()
	ctx := context.Background()

	finding := &models.CodeFinding{
		ID:            uuid.New(),
		Fingerprint:   "fp-crypto-weak-hash-123",
		Title:         "OWASP-A02-CRYPTO-MD5",
		Category:      "security",
		SuggestedDiff: "h := md5.New()",
		Description:   "MD5 hash used for non-cryptographic checksum",
	}

	// 1. Record developer dismissal as FALSE_POSITIVE
	err := svc.RecordDismissal(ctx, wsID, finding, "FALSE_POSITIVE", "lead@company.com")
	if err != nil {
		t.Fatalf("RecordDismissal failed: %v", err)
	}

	// 2. Candidate finding with identical fingerprint should be suppressed immediately
	decision := svc.CheckSuppression(ctx, wsID, finding)
	if !decision.ShouldSuppress {
		t.Fatalf("expected finding to be suppressed via exact fingerprint match")
	}
	if !decision.IsExactMatch {
		t.Errorf("expected IsExactMatch true")
	}
	if decision.SimilarityScore != 1.0 {
		t.Errorf("expected similarity 1.0, got %f", decision.SimilarityScore)
	}
}

func TestSemanticFeedbackService_SemanticVectorSuppression(t *testing.T) {
	store := newMockSecurityMemoryStore()
	embedder := embedding.NewDeterministicSemanticEmbedder()
	svc := feedback.NewSemanticFeedbackService(store, embedder)
	svc.SetDistanceThreshold(0.35)

	wsID := uuid.New()
	ctx := context.Background()

	// Finding dismissed on PR #10
	dismissedFinding := &models.CodeFinding{
		ID:            uuid.New(),
		Fingerprint:   "fp-sql-fmt-1",
		Title:         "SEC-SQL-INJECTION",
		Category:      "security",
		SuggestedDiff: "db.Query(fmt.Sprintf(\"SELECT * FROM items WHERE user_id = '%s'\", id))",
		Description:   "Unescaped string formatting in SQL query",
	}
	_ = svc.RecordDismissal(ctx, wsID, dismissedFinding, "FALSE_POSITIVE", "dev@company.com")

	// New finding on PR #15: slightly different variable name, different fingerprint
	candidateFinding := &models.CodeFinding{
		ID:            uuid.New(),
		Fingerprint:   "fp-sql-fmt-different-fingerprint-2",
		Title:         "SEC-SQL-INJECTION",
		Category:      "security",
		SuggestedDiff: "db.Query(fmt.Sprintf(\"SELECT * FROM items WHERE user_id = '%s'\", userID))",
		Description:   "Unescaped string formatting in SQL query",
	}

	decision := svc.CheckSuppression(ctx, wsID, candidateFinding)
	if !decision.ShouldSuppress {
		t.Fatalf("expected candidate finding to be suppressed via semantic vector memory")
	}
	if decision.IsExactMatch {
		t.Errorf("expected semantic match, not exact match")
	}
	if decision.SimilarityScore < 0.65 {
		t.Errorf("expected high similarity, got %f", decision.SimilarityScore)
	}

	// Completely unrelated finding should NOT be suppressed
	unrelatedFinding := &models.CodeFinding{
		ID:            uuid.New(),
		Fingerprint:   "fp-mem-buffer-leak",
		Title:         "MEM-BUFFER-POOL",
		Category:      "performance",
		SuggestedDiff: "buf := make([]byte, 1024*1024)",
		Description:   "Excessive allocation without sync.Pool",
	}

	unrelatedDecision := svc.CheckSuppression(ctx, wsID, unrelatedFinding)
	if unrelatedDecision.ShouldSuppress {
		t.Errorf("unrelated finding should NOT be suppressed")
	}
}
