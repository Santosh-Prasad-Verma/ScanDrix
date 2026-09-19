package semantic_test

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/semantic"
	"github.com/scandrix/backend/pkg/models"
)

// formatVectorPG converts a float32 or float64 slice to pgvector string format "[x,y,z]"
func formatVectorPG(v []float64) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, val := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(val, 'f', 6, 64))
	}
	sb.WriteByte(']')
	return sb.String()
}

// parseVectorPG parses a pgvector string format "[x,y,z]" into []float64
func parseVectorPG(s string) ([]float64, error) {
	trimmed := strings.Trim(s, "[] \t\r\n")
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, ",")
	vec := make([]float64, len(parts))
	for i, p := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid vector element %q at %d: %w", p, i, err)
		}
		vec[i] = val
	}
	return vec, nil
}

// pgvectorCosineDistance simulates PostgreSQL pgvector `<=>` operator: 1 - cosine_similarity
func pgvectorCosineDistance(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 1.0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA <= 0 || normB <= 0 {
		return 1.0
	}
	sim := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	if sim > 1.0 {
		sim = 1.0
	} else if sim < -1.0 {
		sim = -1.0
	}
	return 1.0 - sim
}

func TestVectorParity_PGVectorSerializationRoundTrip(t *testing.T) {
	dim := 1536
	original := make([]float64, dim)
	for i := 0; i < dim; i++ {
		original[i] = math.Sin(float64(i+1)) / 10.0
	}

	pgStr := formatVectorPG(original)
	if !strings.HasPrefix(pgStr, "[") || !strings.HasSuffix(pgStr, "]") {
		t.Fatalf("expected pgvector format [x,y,...], got %s", pgStr[:20])
	}

	parsed, err := parseVectorPG(pgStr)
	if err != nil {
		t.Fatalf("failed parsing pgvector string: %v", err)
	}

	if len(parsed) != dim {
		t.Fatalf("expected parsed dimension %d, got %d", dim, len(parsed))
	}

	for i := 0; i < dim; i++ {
		if math.Abs(parsed[i]-original[i]) > 1e-5 {
			t.Fatalf("drift at dim %d: original %f, parsed %f", i, original[i], parsed[i])
		}
	}
}

func TestVectorParity_CosineDistanceEquivalence(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()
	dim := 1536

	v1 := generateUnitVector(dim, 42, 0.01)
	v2 := generateUnitVector(dim, 42, 0.05)

	engineDist := engine.CosineDistance(v1, v2)
	pgDist := pgvectorCosineDistance(v1, v2)

	if math.Abs(engineDist-pgDist) > 1e-9 {
		t.Fatalf("cosine distance discrepancy between engine (%f) and pgvector operator (%f)", engineDist, pgDist)
	}
}

func TestVectorParity_HNSWThresholdBoundaryConditions(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()
	dim := 1536

	base := generateUnitVector(dim, 10, 0.0)

	// In pgvector HNSW index, threshold 0.40 distance corresponds to >= 0.60 cosine similarity
	// Create near vector (sim ~0.97, dist ~0.03)
	near := make([]float64, dim)
	copy(near, base)
	near[11] = 0.25
	var norm float64
	for _, v := range near {
		norm += v * v
	}
	invNorm := 1.0 / math.Sqrt(norm)
	for i := range near {
		near[i] *= invNorm
	}

	nearDist := engine.CosineDistance(base, near)
	nearSim := engine.CosineSimilarity(base, near)

	if nearDist >= 0.40 {
		t.Fatalf("expected near vector distance < 0.40, got %f", nearDist)
	}
	if nearSim < 0.60 {
		t.Fatalf("expected near vector similarity >= 0.60, got %f", nearSim)
	}

	// Create far vector (orthogonal: sim 0.0, dist 1.0)
	far := generateUnitVector(dim, 20, 0.0)
	farDist := engine.CosineDistance(base, far)
	farSim := engine.CosineSimilarity(base, far)

	if farDist < 0.99 {
		t.Fatalf("expected orthogonal vector distance ~1.0, got %f", farDist)
	}
	if farSim > 0.01 {
		t.Fatalf("expected orthogonal vector similarity ~0.0, got %f", farSim)
	}
}

func TestVectorParity_HighDimensionalKMeansConvergence(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()
	dim := 1536
	numClusters := 4
	samplesPerCluster := 15
	totalSamples := numClusters * samplesPerCluster

	suggestions := make([]*semantic.EmbeddedSuggestion, totalSamples)
	wsID := uuid.New()

	for c := 0; c < numClusters; c++ {
		for s := 0; s < samplesPerCluster; s++ {
			idx := c*samplesPerCluster + s
			fb := semantic.FeedbackPositiveReaction
			if c%2 == 1 {
				fb = semantic.FeedbackNegativeReaction
			}

			// Primary direction along dimension c*100
			vec := generateUnitVector(dim, c*100, 0.01)
			suggestions[idx] = &semantic.EmbeddedSuggestion{
				ID:           uuid.New(),
				WorkspaceID:  wsID,
				Title:        fmt.Sprintf("Cluster-%d Finding-%d", c, s),
				FeedbackType: fb,
				Embedding:    vec,
			}
		}
	}

	clusterized, centroids, err := engine.ClusterizeSuggestions(suggestions)
	if err != nil {
		t.Fatalf("high-dimensional clustering failed: %v", err)
	}

	if len(clusterized) != totalSamples {
		t.Fatalf("expected %d clusterized items, got %d", totalSamples, len(clusterized))
	}

	// Verify total inertia is small indicating strong convergence
	var totalInertia float64
	for _, cm := range centroids {
		totalInertia += cm.Inertia
	}
	if totalInertia > 5.0 {
		t.Errorf("expected tight cluster inertia (< 5.0), got %f", totalInertia)
	}
}

func TestVectorParity_MultiTenantIsolationSimulation(t *testing.T) {
	ctx := context.Background()
	engine := semantic.NewClusterTuningEngine()
	dim := 256

	wsA := uuid.New()
	wsB := uuid.New()

	// Tenant A dismissed findings (negative)
	historicalA := []*semantic.EmbeddedSuggestion{
		{
			ID:           uuid.New(),
			WorkspaceID:  wsA,
			Title:        "Tenant A false positive",
			FeedbackType: semantic.FeedbackNegativeReaction,
			Embedding:    generateUnitVector(dim, 5, 0.01),
		},
		{
			ID:           uuid.New(),
			WorkspaceID:  wsA,
			Title:        "Tenant A dismissed comment",
			FeedbackType: semantic.FeedbackDismissed,
			Embedding:    generateUnitVector(dim, 5, 0.02),
		},
	}

	// Tenant B accepted findings (positive)
	historicalB := []*semantic.EmbeddedSuggestion{
		{
			ID:           uuid.New(),
			WorkspaceID:  wsB,
			Title:        "Tenant B accepted rule",
			FeedbackType: semantic.FeedbackSuggestionImplemented,
			Embedding:    generateUnitVector(dim, 5, 0.01),
		},
		{
			ID:           uuid.New(),
			WorkspaceID:  wsB,
			Title:        "Tenant B positive fix",
			FeedbackType: semantic.FeedbackPositiveReaction,
			Embedding:    generateUnitVector(dim, 5, 0.02),
		},
	}

	candidate := models.CodeFinding{
		ID:          uuid.New(),
		WorkspaceID: wsA,
		Title:       "Test finding",
		Category:    "LOGIC",
	}
	candEmb := generateUnitVector(dim, 5, 0.01)

	// Evaluated against Tenant A history -> Should be DISCARDED
	keptA, discardedA, _, err := engine.FilterFindingsBatch(ctx, []models.CodeFinding{candidate}, [][]float64{candEmb}, historicalA)
	if err != nil {
		t.Fatalf("filter A failed: %v", err)
	}
	if len(discardedA) != 1 || len(keptA) != 0 {
		t.Fatalf("expected finding discarded under Tenant A history, got kept: %d, discarded: %d", len(keptA), len(discardedA))
	}

	// Evaluated against Tenant B history -> Should be KEPT
	candidate.WorkspaceID = wsB
	keptB, discardedB, _, err := engine.FilterFindingsBatch(ctx, []models.CodeFinding{candidate}, [][]float64{candEmb}, historicalB)
	if err != nil {
		t.Fatalf("filter B failed: %v", err)
	}
	if len(keptB) != 1 || len(discardedB) != 0 {
		t.Fatalf("expected finding kept under Tenant B history, got kept: %d, discarded: %d", len(keptB), len(discardedB))
	}
}

func TestVectorParity_ConcurrentHighThroughputMath(t *testing.T) {
	engine := semantic.NewClusterTuningEngine()
	dim := 1536
	workers := 24
	iterations := 1000

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			vA := generateUnitVector(dim, workerID%dim, 0.05)
			vB := generateUnitVector(dim, (workerID+1)%dim, 0.05)

			for i := 0; i < iterations; i++ {
				sim := engine.CosineSimilarity(vA, vB)
				dist := engine.CosineDistance(vA, vB)
				if math.Abs((sim+dist)-1.0) > 1e-7 {
					t.Errorf("worker %d math invariant violated: sim %f + dist %f != 1.0", workerID, sim, dist)
					return
				}
			}
		}(w)
	}

	wg.Wait()
}
