// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package providers

import (
	"testing"

	_ "github.com/scandrix/backend/internal/llm/providers/all"
)

func TestIsCuratedCatalogProvider(t *testing.T) {
	// Unknown provider
	if IsCuratedCatalogProvider("not-a-provider") {
		t.Errorf("expected false for unregistered provider")
	}
	if IsCuratedCatalogProvider("") {
		t.Errorf("expected false for empty provider")
	}

	// Providers with static / curated modelListing (Bedrock and Vertex)
	if !IsCuratedCatalogProvider("amazon_bedrock") {
		t.Errorf("expected true for amazon_bedrock curated catalog")
	}
	if !IsCuratedCatalogProvider("google_vertex") {
		t.Errorf("expected true for google_vertex curated catalog")
	}
}
