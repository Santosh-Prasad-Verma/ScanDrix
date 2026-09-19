// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package embedding_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/llm/embedding"
)

func TestDeterministicSemanticEmbedder_DimensionsAndNormalization(t *testing.T) {
	embedder := embedding.NewDeterministicSemanticEmbedder()

	if embedder.Dimension() != embedding.VectorDimension {
		t.Fatalf("expected dimension %d, got %d", embedding.VectorDimension, embedder.Dimension())
	}

	snippet := "func validateUser(token string) bool { return token != \"\" }"
	vec, err := embedder.EmbedText(context.Background(), snippet)
	if err != nil {
		t.Fatalf("EmbedText failed: %v", err)
	}

	if len(vec) != embedding.VectorDimension {
		t.Fatalf("expected vector length %d, got %d", embedding.VectorDimension, len(vec))
	}

	// Verify L2 norm is 1.0 (within epsilon)
	var sum float64
	for _, x := range vec {
		sum += float64(x * x)
	}
	norm := math.Sqrt(sum)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("expected unit L2 norm 1.0, got %f", norm)
	}
}

func TestDeterministicSemanticEmbedder_SemanticSimilarity(t *testing.T) {
	embedder := embedding.NewDeterministicSemanticEmbedder()
	ctx := context.Background()

	// Similar code snippets with slight variable renames / formatting
	codeA := "query := fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", userID)"
	codeB := "sql := fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", id)"

	// Completely unrelated code
	codeC := "buffer := bytes.NewBuffer(make([]byte, 1024))\ncopy(buffer.Bytes(), payload)"

	vecA, _ := embedder.EmbedText(ctx, codeA)
	vecB, _ := embedder.EmbedText(ctx, codeB)
	vecC, _ := embedder.EmbedText(ctx, codeC)

	distSimilar := embedding.CosineDistance(vecA, vecB)
	distUnrelated := embedding.CosineDistance(vecA, vecC)

	// Verify similar code has smaller cosine distance (< 0.35) than unrelated code
	if distSimilar >= distUnrelated {
		t.Errorf("expected distSimilar (%f) < distUnrelated (%f)", distSimilar, distUnrelated)
	}
	if distSimilar > 0.35 {
		t.Errorf("expected close distance for similar SQL injections, got %f", distSimilar)
	}
}

func TestOpenAIEmbedder_MockServer(t *testing.T) {
	mockVec := make([]float32, embedding.VectorDimension)
	for i := range mockVec {
		mockVec[i] = 0.02
	}
	mockVec = embedding.NormalizeVector(mockVec)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"index":     0,
					"embedding": mockVec,
				},
			},
		})
	}))
	defer server.Close()

	embedder := embedding.NewOpenAIEmbedder("test-api-key", server.URL, "text-embedding-3-small", server.Client())
	vec, err := embedder.EmbedText(context.Background(), "SELECT * FROM accounts")
	if err != nil {
		t.Fatalf("OpenAIEmbedder failed: %v", err)
	}

	if len(vec) != embedding.VectorDimension {
		t.Fatalf("expected dimension %d, got %d", embedding.VectorDimension, len(vec))
	}
}

func TestResilientEmbedder_FallbackOnFailure(t *testing.T) {
	// Failing server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	primary := embedding.NewOpenAIEmbedder("bad-key", server.URL, "text-embedding-3-small", server.Client())
	resilient := embedding.NewResilientEmbedder(primary)

	vec, err := resilient.EmbedText(context.Background(), "log.Println(\"testing fallback\")")
	if err != nil {
		t.Fatalf("resilient embedder should not error on primary failure: %v", err)
	}

	if len(vec) != embedding.VectorDimension {
		t.Fatalf("expected 1536 floats from fallback, got %d", len(vec))
	}
}
