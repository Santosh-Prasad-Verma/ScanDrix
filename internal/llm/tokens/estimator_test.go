// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tokens_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/tokens"
)

func TestEstimateTextTokens(t *testing.T) {
	if tokens.EstimateTextTokens("") != 0 {
		t.Fatal("expected 0 for empty string")
	}
	if tokens.EstimateTextTokens("abc") != 1 {
		t.Fatalf("expected 1 token for 3 chars, got %d", tokens.EstimateTextTokens("abc"))
	}
	if tokens.EstimateTextTokens("abcdef") != 2 {
		t.Fatalf("expected 2 tokens for 6 chars, got %d", tokens.EstimateTextTokens("abcdef"))
	}
	if tokens.EstimateTextTokens("abcd") != 2 {
		t.Fatalf("expected 2 tokens for 4 chars (ceil), got %d", tokens.EstimateTextTokens("abcd"))
	}
}
