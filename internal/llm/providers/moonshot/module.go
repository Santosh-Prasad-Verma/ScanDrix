// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package moonshot

import (
	"context"
	"net/http"
	"strings"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/anthropic"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// Module implements the Moonshot (Kimi) provider module for ScanDrix.
type Module struct {
	anthropicModule *anthropic.Module
}

type Option func(*Module)

func WithHTTPClient(client *http.Client) Option {
	return func(m *Module) {
		m.anthropicModule = anthropic.New(anthropic.WithHTTPClient(client))
	}
}

func New(opts ...Option) *Module {
	m := &Module{
		anthropicModule: anthropic.New(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Module) ID() string {
	return "moonshot"
}

func (m *Module) Aliases() []string {
	return []string{}
}

func (m *Module) Label() string {
	return "Moonshot"
}

func (m *Module) Doc() string {
	return "https://platform.moonshot.ai/docs"
}

func (m *Module) DefaultBaseURL() string {
	return "https://api.moonshot.ai/anthropic"
}

func (m *Module) Capabilities(model string) kernel.ModelCapabilities {
	caps := m.anthropicModule.Capabilities(model)
	caps.SupportsReasoning = true
	return caps
}

func (m *Module) ReasoningTraits(cfg byok.NormalizedModel) kernel.ModelReasoningTraits {
	return kernel.ModelReasoningTraits{
		ThinksByDefault:                true,
		CanDisableThinking:             false,
		ForcedToolChoiceSupported:      true,
		RejectsThinkingWhenToolsForced: false,
		BudgetMode:                     "fixed",
	}
}

func (m *Module) TemperaturePolicy(cfg byok.NormalizedModel) *kernel.TemperaturePolicy {
	fixed := 1.0
	return &kernel.TemperaturePolicy{
		Mode:       "fixed",
		FixedValue: &fixed,
	}
}

func (m *Module) SystemCacheControl(cfg byok.NormalizedModel) map[string]any {
	return nil
}

func (m *Module) UIFields() []kernel.FieldDescriptor {
	return []kernel.FieldDescriptor{
		{Key: "apiKey", Label: "API Key", Type: "password", Required: true, Scope: "top"},
		{Key: "baseURL", Label: "Base URL", Type: "url", Required: false, Scope: "top", Placeholder: "https://api.moonshot.ai/anthropic"},
	}
}

func (m *Module) ModelListing(providerID string) *kernel.ModelListing {
	if providerID == "moonshot" {
		return &kernel.ModelListing{
			Kind:      kernel.ListingHTTP,
			APIKeyEnv: "API_MOONSHOT_API_KEY",
			TimeoutMs: 15000,
			URL: func(creds kernel.ResolvedListingCreds) string {
				return "https://api.moonshot.ai/v1/models"
			},
			Headers: func(creds kernel.ResolvedListingCreds) map[string]string {
				return kernel.BearerHeaders(creds.APIKey)
			},
			Parse: kernel.ParseOpenAIIDs,
		}
	}
	return nil
}

func (m *Module) Execute(ctx context.Context, cfg byok.NormalizedModel, req kernel.ExecutionRequest) (*kernel.ExecutionResult, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = m.DefaultBaseURL()
	}
	return m.anthropicModule.Execute(ctx, cfg, req)
}
