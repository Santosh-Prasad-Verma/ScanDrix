// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgparamusecases

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ModelDescriptor represents a model selectable in the catalog.
type ModelDescriptor struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Provider    string `json:"provider"`
	Tier        string `json:"tier"` // flagship, fast, reasoning
}

// GetModelsByProviderUseCase returns the supported models for a target provider dynamically via the provider registry.
type GetModelsByProviderUseCase struct {
	httpClient *http.Client
}

func NewGetModelsByProviderUseCase() *GetModelsByProviderUseCase {
	return &GetModelsByProviderUseCase{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (uc *GetModelsByProviderUseCase) Execute(provider string) []ModelDescriptor {
	provKey := strings.ToLower(strings.TrimSpace(provider))
	mod, ok := kernel.Get(provKey)
	if !ok {
		return defaultFallbackModels(provKey)
	}

	listing := mod.ModelListing(provKey)
	if listing == nil {
		return defaultFallbackModels(provKey)
	}

	var catalogModels []kernel.CatalogModel

	switch listing.Kind {
	case kernel.ListingStatic:
		catalogModels = listing.StaticModels

	case kernel.ListingHTTP:
		// Attempt live HTTP listing if API key is present in environment or credentials
		apiKey := ""
		if listing.APIKeyEnv != "" {
			apiKey = strings.TrimSpace(os.Getenv(listing.APIKeyEnv))
		}
		baseURL := listing.DefaultBaseURL
		if listing.BaseURLEnv != "" && os.Getenv(listing.BaseURLEnv) != "" {
			baseURL = strings.TrimSpace(os.Getenv(listing.BaseURLEnv))
		}

		if (apiKey != "" || !listing.RequiresBaseURL) && listing.URL != nil && listing.Parse != nil {
			creds := kernel.ResolvedListingCreds{
				APIKey:  apiKey,
				BaseURL: baseURL,
			}
			reqURL := listing.URL(creds)
			if reqURL != "" {
				req, err := http.NewRequest(http.MethodGet, reqURL, nil)
				if err == nil {
					if listing.Headers != nil {
						for k, v := range listing.Headers(creds) {
							req.Header.Set(k, v)
						}
					}
					resp, err := uc.httpClient.Do(req)
					if err == nil {
						defer resp.Body.Close()
						if resp.StatusCode >= 200 && resp.StatusCode < 300 {
							body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
							if err == nil {
								parsed, err := listing.Parse(body)
								if err == nil && len(parsed) > 0 {
									catalogModels = parsed
								}
							}
						}
					}
				}
			}
		}

		if len(catalogModels) == 0 {
			catalogModels = listing.FallbackModels
		}

	case kernel.ListingManual:
		catalogModels = listing.FallbackModels
	}

	if len(catalogModels) == 0 {
		return defaultFallbackModels(provKey)
	}

	descriptors := make([]ModelDescriptor, 0, len(catalogModels))
	for _, m := range catalogModels {
		tier := "flagship"
		idLower := strings.ToLower(m.ID)
		if m.SupportsReasoning || strings.Contains(idLower, "reasoning") || strings.Contains(idLower, "o1") || strings.Contains(idLower, "o3") || strings.Contains(idLower, "r1") {
			tier = "reasoning"
		} else if strings.Contains(idLower, "mini") || strings.Contains(idLower, "flash") || strings.Contains(idLower, "haiku") || strings.Contains(idLower, "small") {
			tier = "fast"
		}

		name := m.Name
		if name == "" {
			name = m.ID
		}

		descriptors = append(descriptors, ModelDescriptor{
			ID:          m.ID,
			DisplayName: name,
			Provider:    provKey,
			Tier:        tier,
		})
	}

	return descriptors
}

func defaultFallbackModels(provider string) []ModelDescriptor {
	switch provider {
	case "anthropic":
		return []ModelDescriptor{
			{ID: "claude-3-7-sonnet", DisplayName: "Claude 3.7 Sonnet (Hybrid Reasoning)", Provider: "anthropic", Tier: "flagship"},
			{ID: "claude-3-5-sonnet", DisplayName: "Claude 3.5 Sonnet", Provider: "anthropic", Tier: "flagship"},
			{ID: "claude-3-5-haiku", DisplayName: "Claude 3.5 Haiku", Provider: "anthropic", Tier: "fast"},
		}
	case "openai":
		return []ModelDescriptor{
			{ID: "gpt-4o", DisplayName: "GPT-4o", Provider: "openai", Tier: "flagship"},
			{ID: "gpt-4o-mini", DisplayName: "GPT-4o Mini", Provider: "openai", Tier: "fast"},
			{ID: "o3-mini", DisplayName: "o3 Mini", Provider: "openai", Tier: "reasoning"},
		}
	case "gemini", "google":
		return []ModelDescriptor{
			{ID: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", Provider: "gemini", Tier: "fast"},
			{ID: "gemini-2.0-pro-exp", DisplayName: "Gemini 2.0 Pro", Provider: "gemini", Tier: "flagship"},
		}
	case "vertex":
		return []ModelDescriptor{
			{ID: "claude-3-5-sonnet@20241022", DisplayName: "Claude 3.5 Sonnet on Vertex", Provider: "vertex", Tier: "flagship"},
			{ID: "gemini-2.0-flash-001", DisplayName: "Gemini 2.0 Flash on Vertex", Provider: "vertex", Tier: "fast"},
		}
	case "bedrock":
		return []ModelDescriptor{
			{ID: "anthropic.claude-3-5-sonnet-20241022-v2:0", DisplayName: "Claude 3.5 Sonnet on Bedrock", Provider: "bedrock", Tier: "flagship"},
		}
	default:
		return []ModelDescriptor{
			{ID: "default-model", DisplayName: "Default Model", Provider: provider, Tier: "flagship"},
		}
	}
}
