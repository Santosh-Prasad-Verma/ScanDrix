// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"testing"
)

func TestManagedModelMaxInputTokens(t *testing.T) {
	t.Run("resolves the managed Gemini window from the registry (google -> google_gemini)", func(t *testing.T) {
		tokens := ManagedModelMaxInputTokens("google:gemini-2.5-pro")
		if tokens == nil {
			t.Fatalf("expected non-nil tokens for google:gemini-2.5-pro")
		}
		if *tokens != 1_000_000 {
			t.Fatalf("expected 1,000,000, got %d", *tokens)
		}
	})

	t.Run("resolves the Gemini 3.1 flash-lite window", func(t *testing.T) {
		tokens := ManagedModelMaxInputTokens("google:gemini-3.1-flash-lite-preview")
		if tokens == nil {
			t.Fatalf("expected non-nil tokens for google:gemini-3.1-flash-lite-preview")
		}
		if *tokens != 1_048_576 {
			t.Fatalf("expected 1,048,576, got %d", *tokens)
		}
	})

	t.Run("resolves the legacy Claude-on-Vertex window (vertex -> google_vertex)", func(t *testing.T) {
		tokens := ManagedModelMaxInputTokens("vertex:claude-3-5-sonnet")
		if tokens == nil {
			t.Fatalf("expected non-nil tokens for vertex:claude-3-5-sonnet")
		}
		if *tokens != 200_000 {
			t.Fatalf("expected 200,000, got %d", *tokens)
		}
	})

	t.Run("returns nil for a managed model with no pinned window", func(t *testing.T) {
		tokens := ManagedModelMaxInputTokens("google:gemini-2.0-flash")
		if tokens != nil {
			t.Fatalf("expected nil for google:gemini-2.0-flash, got %v", *tokens)
		}
	})

	t.Run("returns nil for a bare BYOK model string (no vendor prefix)", func(t *testing.T) {
		if tokens := ManagedModelMaxInputTokens("gpt-4o"); tokens != nil {
			t.Fatalf("expected nil for bare gpt-4o, got %v", *tokens)
		}
		if tokens := ManagedModelMaxInputTokens("accounts/fireworks/models/deepseek-v4-flash-0731"); tokens != nil {
			t.Fatalf("expected nil for fireworks model without vendor prefix, got %v", *tokens)
		}
	})

	t.Run("returns nil for an unknown vendor prefix or empty input", func(t *testing.T) {
		if tokens := ManagedModelMaxInputTokens("unknown:some-model"); tokens != nil {
			t.Fatalf("expected nil for unknown vendor, got %v", *tokens)
		}
		if tokens := ManagedModelMaxInputTokens(""); tokens != nil {
			t.Fatalf("expected nil for empty string, got %v", *tokens)
		}
		if tokens := ManagedModelMaxInputTokens("   "); tokens != nil {
			t.Fatalf("expected nil for whitespace string, got %v", *tokens)
		}
	})
}
