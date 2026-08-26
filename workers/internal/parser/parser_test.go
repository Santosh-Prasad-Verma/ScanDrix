package parser_test

import (
	"testing"

	"github.com/codehound/codehound/workers/internal/parser"
	"github.com/google/uuid"
)

func TestParserHashAndExtraction(t *testing.T) {
	content := []byte("package main\n\nfunc main() {}\n")
	hash := parser.ComputeHash(content)

	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}

	p := parser.New()
	fileID := uuid.New()
	res := p.ExtractBasicSymbols(fileID, content)

	if len(res.Symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(res.Symbols))
	}
	if res.Symbols[0].Name != "main" {
		t.Fatalf("expected symbol name 'main', got %s", res.Symbols[0].Name)
	}
}
