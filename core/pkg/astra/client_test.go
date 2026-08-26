package astra_test

import (
	"context"
	"testing"

	"github.com/codehound/codehound/core/pkg/astra"
	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestAstraClientAndASTChunking(t *testing.T) {
	client := astra.NewClient("https://astra.datastax.com", "AstraCS:token", "codehound_test")
	ctx := context.Background()

	tenantID := uuid.New()
	projectID := uuid.New()

	code := []byte("package test\n\nfunc Hello() string {\n\treturn \"world\"\n}\n")
	symbols := []domain.CodeSymbol{
		{
			ID:         uuid.New(),
			Name:       "Hello",
			Kind:       "FUNCTION",
			StartLine:  3,
			EndLine:    5,
			SymbolKey:  "test.Hello",
			Visibility: "PUBLIC",
		},
	}

	chunks := astra.ChunkCodeByAST(tenantID, projectID, "pkg/test/hello.go", "go", symbols, code)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if err := client.InsertCodeChunk(ctx, chunks[0]); err != nil {
		t.Fatalf("failed to insert code chunk: %v", err)
	}

	results, err := client.SearchSimilarCode(ctx, tenantID, projectID, 5)
	if err != nil {
		t.Fatalf("failed to search similar code: %v", err)
	}

	if len(results) != 1 || results[0].SymbolName != "Hello" {
		t.Errorf("expected 1 result with symbol Hello, got %+v", results)
	}
}
