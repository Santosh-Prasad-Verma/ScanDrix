package semantic_test

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/semantic"
	"github.com/scandrix/backend/pkg/models"
)

// helper to generate deterministic unit-norm vector in D dimensions
func generateUnitVector(dim int, primaryDim int, perturbation float64) []float64 {
	vec := make([]float64, dim)
	for i := range vec {
		vec[i] = perturbation * math.Sin(float64(i+1))
	}
	if primaryDim >= 0 && primaryDim < dim {
		vec[primaryDim] += 1.0
	}
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		inv := 1.0 / math.Sqrt(norm)
		for i := range vec {
			vec[i] *= inv
		}
	}
	return vec
}

func TestClusterTuningEngine_CosineSimilarityPrecision(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()

	// 1. Identical vectors -> similarity = 1.0
	v1 := []float64{1.0, 2.0, 3.0}
	sim := engine.CosineSimilarity(v1, v1)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Fatalf("expected identical vectors to have similarity 1.0, got %f", sim)
	}

	// 2. Orthogonal vectors -> similarity = 0.0
	v2 := []float64{1.0, 0.0, 0.0}
	v3 := []float64{0.0, 1.0, 0.0}
	sim = engine.CosineSimilarity(v2, v3)
	if math.Abs(sim-0.0) > 1e-6 {
		t.Fatalf("expected orthogonal vectors to have similarity 0.0, got %f", sim)
	}

	// 3. Opposite vectors -> similarity = -1.0
	v4 := []float64{-1.0, 0.0, 0.0}
	sim = engine.CosineSimilarity(v2, v4)
	if math.Abs(sim-(-1.0)) > 1e-6 {
		t.Fatalf("expected opposite vectors to have similarity -1.0, got %f", sim)
	}

	// 4. Zero vector handling
	vZero := []float64{0.0, 0.0, 0.0}
	sim = engine.CosineSimilarity(v1, vZero)
	if sim != 0.0 {
		t.Fatalf("expected zero vector similarity to be 0.0, got %f", sim)
	}
}

func TestClusterTuningEngine_TextNormalization(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()

	raw := "  Fix: Possible SQL-Injection in User_Repository (Line 42)!  "
	normalized := engine.NormalizeText(raw)
	expected := "fix possible sql-injection in user_repository line 42"
	if normalized != expected {
		t.Fatalf("expected %q, got %q", expected, normalized)
	}
}

func TestClusterTuningEngine_FeedbackClassification(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()

	// Implemented wins over reactions
	fb := engine.ClassifyFeedback(0, 10, true, false)
	if fb != semantic.FeedbackSuggestionImplemented {
		t.Fatalf("expected FeedbackSuggestionImplemented, got %v", fb)
	}

	// Dismissed wins over positive
	fb = engine.ClassifyFeedback(10, 0, false, true)
	if fb != semantic.FeedbackDismissed {
		t.Fatalf("expected FeedbackDismissed, got %v", fb)
	}

	// Thumbs up majority
	fb = engine.ClassifyFeedback(5, 1, false, false)
	if fb != semantic.FeedbackPositiveReaction {
		t.Fatalf("expected FeedbackPositiveReaction, got %v", fb)
	}

	// Thumbs down majority
	fb = engine.ClassifyFeedback(1, 5, false, false)
	if fb != semantic.FeedbackNegativeReaction {
		t.Fatalf("expected FeedbackNegativeReaction, got %v", fb)
	}

	// Equal -> Neutral
	fb = engine.ClassifyFeedback(2, 2, false, false)
	if fb != semantic.FeedbackNeutral {
		t.Fatalf("expected FeedbackNeutral, got %v", fb)
	}
}

func TestClusterTuningEngine_ClusteringConvergence(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()
	dim := 1536
	n := 40

	// Generate synthetic clusters: 20 around dim 0 (positive), 20 around dim 1 (negative)
	historical := make([]*semantic.EmbeddedSuggestion, n)
	wsID := uuid.New()

	for i := 0; i < 20; i++ {
		historical[i] = &semantic.EmbeddedSuggestion{
			ID:           uuid.New(),
			WorkspaceID:  wsID,
			Title:        "Resource leak fix",
			FeedbackType: semantic.FeedbackSuggestionImplemented,
			Embedding:    generateUnitVector(dim, 0, 0.05),
			CreatedAt:    time.Now().UTC(),
		}
	}
	for i := 20; i < 40; i++ {
		historical[i] = &semantic.EmbeddedSuggestion{
			ID:           uuid.New(),
			WorkspaceID:  wsID,
			Title:        "Unwanted stylistic comment",
			FeedbackType: semantic.FeedbackNegativeReaction,
			Embedding:    generateUnitVector(dim, 1, 0.05),
			CreatedAt:    time.Now().UTC(),
		}
	}

	clusterized, centroids, err := engine.ClusterizeSuggestions(historical)
	if err != nil {
		t.Fatalf("clustering failed: %v", err)
	}

	if len(clusterized) != n {
		t.Fatalf("expected %d clusterized items, got %d", n, len(clusterized))
	}
	if len(centroids) < 2 {
		t.Fatalf("expected at least 2 centroids, got %d", len(centroids))
	}

	// Verify all items have valid cluster assignment
	for i, cs := range clusterized {
		if cs.Cluster < 0 || cs.Cluster >= len(centroids) {
			t.Fatalf("invalid cluster index %d for item %d", cs.Cluster, i)
		}
		if cs.SimilarityToCentroid < 0.5 {
			t.Errorf("item %d has low similarity to centroid: %f", i, cs.SimilarityToCentroid)
		}
	}
}

func TestClusterTuningEngine_DecisionDecoupling(t *testing.T) {
	ctx := context.Background()
	engine := semantic.NewClusterTuningEngine()
	dim := 128

	// Create 2 clusters: Cluster 0 is accepted fixes, Cluster 1 is dismissed false-positives
	historical := make([]*semantic.EmbeddedSuggestion, 20)
	for i := 0; i < 10; i++ {
		historical[i] = &semantic.EmbeddedSuggestion{
			ID:           uuid.New(),
			Title:        "Safe fix",
			FeedbackType: semantic.FeedbackPositiveReaction,
			Embedding:    generateUnitVector(dim, 0, 0.02),
		}
	}
	for i := 10; i < 20; i++ {
		historical[i] = &semantic.EmbeddedSuggestion{
			ID:           uuid.New(),
			Title:        "False alarm",
			FeedbackType: semantic.FeedbackNegativeReaction,
			Embedding:    generateUnitVector(dim, 1, 0.02),
		}
	}

	// Candidate 1: matches positive cluster -> KEEP
	candPositive := models.CodeFinding{
		ID:          uuid.New(),
		Title:       "Close body on response",
		Category:    "BUG",
		Description: "Prevent socket leak",
	}
	embPositive := generateUnitVector(dim, 0, 0.02)

	// Candidate 2: matches negative cluster -> DISCARD
	candNegative := models.CodeFinding{
		ID:          uuid.New(),
		Title:       "Style comment nitpick",
		Category:    "STYLE",
		Description: "Trivial comment",
	}
	embNegative := generateUnitVector(dim, 1, 0.02)

	// Candidate 3: security finding matching negative cluster -> KEEP (Rule safety override)
	candSecurity := models.CodeFinding{
		ID:          uuid.New(),
		Title:       "SQL Injection vulnerability",
		Category:    "SECURITY",
		Description: "Critical flaw",
	}
	embSecurity := generateUnitVector(dim, 1, 0.02) // even if vector is near negative cluster!

	findings := []models.CodeFinding{candPositive, candNegative, candSecurity}
	embeddings := [][]float64{embPositive, embNegative, embSecurity}

	kept, discarded, results, err := engine.FilterFindingsBatch(ctx, findings, embeddings, historical)
	if err != nil {
		t.Fatalf("filter batch failed: %v", err)
	}

	if len(kept) != 2 {
		t.Fatalf("expected 2 kept findings (positive + security override), got %d", len(kept))
	}
	if len(discarded) != 1 {
		t.Fatalf("expected 1 discarded finding (stylistic nitpick), got %d", len(discarded))
	}
	if discarded[0].Title != "Style comment nitpick" {
		t.Errorf("unexpected discarded title: %s", discarded[0].Title)
	}

	// Verify security rule was kept
	foundSec := false
	for _, k := range kept {
		if k.Category == "SECURITY" {
			foundSec = true
		}
	}
	if !foundSec {
		t.Errorf("expected security finding to be preserved by safety override")
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestClusterTuningEngine_ConcurrentExecutionUnderLoad(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()
	dim := 64
	numWorkers := 16

	// Historical base set
	historical := make([]*semantic.EmbeddedSuggestion, 30)
	for i := 0; i < 15; i++ {
		historical[i] = &semantic.EmbeddedSuggestion{
			ID:           uuid.New(),
			FeedbackType: semantic.FeedbackPositiveReaction,
			Embedding:    generateUnitVector(dim, 0, 0.05),
		}
	}
	for i := 15; i < 30; i++ {
		historical[i] = &semantic.EmbeddedSuggestion{
			ID:           uuid.New(),
			FeedbackType: semantic.FeedbackNegativeReaction,
			Embedding:    generateUnitVector(dim, 1, 0.05),
		}
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()

			finding := models.CodeFinding{
				ID:          uuid.New(),
				Title:       "Worker test finding",
				Category:    "LOGIC",
				Description: "Concurrency validation",
			}
			emb := generateUnitVector(dim, workerID%2, 0.05)

			kept, discarded, _, err := engine.FilterFindingsBatch(ctx, []models.CodeFinding{finding}, [][]float64{emb}, historical)
			if err != nil {
				t.Errorf("worker %d filter error: %v", workerID, err)
			}
			if len(kept)+len(discarded) != 1 {
				t.Errorf("worker %d expected 1 result", workerID)
			}
		}(w)
	}

	wg.Wait()
}
