// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package providers

import (
	"testing"

	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestDescribeProviderID_OpenAI(t *testing.T) {
	mod, ok := kernel.DefaultRegistry.Get("openai")
	if !ok {
		t.Fatalf("openai provider not registered")
	}

	desc := DescribeProviderID(mod, "openai")
	if desc.ID != "openai" {
		t.Errorf("expected ID openai, got %s", desc.ID)
	}
	if !desc.RequiresAPIKey {
		t.Errorf("expected OpenAI to require API key")
	}
	if desc.RequiresBaseURL {
		t.Errorf("native OpenAI should not require base URL")
	}
	if !desc.AutoListModels {
		t.Errorf("OpenAI models should be auto-listable")
	}
	if !desc.ListsModelsLive {
		t.Errorf("OpenAI models should be listed live via HTTP")
	}
}

func TestDescribeProviderID_Compatible(t *testing.T) {
	mod, ok := kernel.DefaultRegistry.Get("openai")
	if !ok {
		t.Fatalf("openai provider not registered")
	}

	desc := DescribeProviderID(mod, "openai_compatible")
	if !IsCustomEndpoint("openai_compatible") {
		t.Errorf("expected openai_compatible to be custom endpoint")
	}
	if !desc.RequiresBaseURL {
		t.Errorf("openai_compatible MUST require base URL")
	}
	if desc.AutoListModels {
		t.Errorf("custom endpoints cannot auto list models before endpoint is known")
	}
	if desc.ListsModelsLive {
		t.Errorf("custom endpoints cannot list live before endpoint is known")
	}
}

func TestDescribeAllProviderIDs(t *testing.T) {
	allMods := kernel.DefaultRegistry.List()
	descriptors := DescribeAllProviderIDs(allMods)

	if len(descriptors) == 0 {
		t.Fatalf("expected descriptors for registered providers, got 0")
	}

	foundNative := false
	foundCompatible := false
	for _, d := range descriptors {
		if d.ID == "openai" {
			foundNative = true
		}
		if d.ID == "openai_compatible" {
			foundCompatible = true
		}
	}

	if !foundNative || !foundCompatible {
		t.Errorf("expected to find both native and compatible descriptors")
	}
}
