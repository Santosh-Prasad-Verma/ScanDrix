// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package kernel

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
)

type dummyProvider struct {
	id      string
	aliases []string
}

func (d *dummyProvider) ID() string                                                    { return d.id }
func (d *dummyProvider) Aliases() []string                                             { return d.aliases }
func (d *dummyProvider) Label() string                                                 { return "Dummy" }
func (d *dummyProvider) Doc() string                                                   { return "https://docs.scandrix.dev" }
func (d *dummyProvider) Capabilities(model string) ModelCapabilities                   { return ModelCapabilities{} }
func (d *dummyProvider) ReasoningTraits(cfg byok.NormalizedModel) ModelReasoningTraits { return ModelReasoningTraits{} }
func (d *dummyProvider) TemperaturePolicy(cfg byok.NormalizedModel) *TemperaturePolicy { return nil }
func (d *dummyProvider) SystemCacheControl(cfg byok.NormalizedModel) map[string]any    { return nil }
func (d *dummyProvider) UIFields() []FieldDescriptor                                  { return nil }
func (d *dummyProvider) ModelListing(providerID string) *ModelListing                  { return nil }
func (d *dummyProvider) Execute(ctx context.Context, cfg byok.NormalizedModel, req ExecutionRequest) (*ExecutionResult, error) {
	return &ExecutionResult{Text: "dummy response"}, nil
}

func TestProviderRegistry(t *testing.T) {
	reg := NewProviderRegistry()
	p := &dummyProvider{id: "openai", aliases: []string{"openai_compatible", "azure_openai"}}

	reg.Register(p)

	if got, ok := reg.Get("openai"); !ok || got.ID() != "openai" {
		t.Fatalf("expected to find openai, got %v, ok=%v", got, ok)
	}

	if got, ok := reg.Get("OPENAI_COMPATIBLE"); !ok || got.ID() != "openai" {
		t.Fatalf("expected to find alias openai_compatible (case-insensitive), got %v, ok=%v", got, ok)
	}

	if _, ok := reg.Get("nonexistent"); ok {
		t.Fatalf("expected nonexistent to return false")
	}

	list := reg.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 unique provider, got %d", len(list))
	}
}
