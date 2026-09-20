package document

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateDocumentAndTokenEstimation(t *testing.T) {
	doc := CreateDocument("Hello world, this is a test document.", map[string]interface{}{
		"source": "readme.md",
		"repoId": "repo-123",
	})

	if doc.PageContent != "Hello world, this is a test document." {
		t.Fatalf("unexpected content: %s", doc.PageContent)
	}

	if doc.Metadata["source"] != "readme.md" {
		t.Fatalf("unexpected metadata: %+v", doc.Metadata)
	}

	tokens := EstimateTokenCount(doc.PageContent)
	if tokens < 5 || tokens > 15 {
		t.Fatalf("unexpected token count: %d", tokens)
	}
}

func TestEmbedderWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{
					"object": "embedding",
					"index": 0,
					"embedding": [0.1, 0.2, 0.3]
				}
			],
			"model": "text-embedding-3-small"
		}`))
	}))
	defer server.Close()

	embedder := BuildPlatformEmbedder(EmbeddingOptions{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	if embedder == nil {
		t.Fatalf("expected non-nil embedder")
	}

	res, err := embedder.Embed(context.Background(), "test text to embed")
	if err != nil {
		t.Fatalf("unexpected error embedding: %v", err)
	}

	if len(res.Data) != 1 || len(res.Data[0].Embedding) != 3 {
		t.Fatalf("unexpected embedding response: %+v", res)
	}
}
