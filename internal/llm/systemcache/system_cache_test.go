// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package systemcache_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/systemcache"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
)

func TestSystemCacheControl_Anthropic(t *testing.T) {
	hint := systemcache.SystemCacheControl(systemcache.SystemCacheControlInput{
		Provider: "anthropic",
		Model:    "claude-3-7-sonnet-20250219",
	})
	if hint == nil {
		t.Fatal("expected non-nil cache control hint for Anthropic")
	}
	if hint["type"] != "ephemeral" {
		t.Fatalf("expected ephemeral cache hint, got %v", hint["type"])
	}
}

func TestSystemCacheControl_FallbackByModelName(t *testing.T) {
	hint := systemcache.SystemCacheControl(systemcache.SystemCacheControlInput{
		Provider: "",
		Model:    "claude-3-5-sonnet-latest",
	})
	if hint == nil {
		t.Fatal("expected non-nil cache control hint for claude model name fallback")
	}
	if hint["type"] != "ephemeral" {
		t.Fatalf("expected ephemeral cache hint, got %v", hint["type"])
	}
}

func TestSystemCacheControl_NonAnthropic(t *testing.T) {
	hint := systemcache.SystemCacheControl(systemcache.SystemCacheControlInput{
		Provider: "openai",
		Model:    "gpt-4o",
	})
	if hint != nil {
		t.Fatalf("expected nil cache hint for OpenAI, got %v", hint)
	}
}
