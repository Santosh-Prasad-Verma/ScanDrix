// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package providers

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ProviderUIDescriptor represents UI-facing provider configuration and capability flags.
type ProviderUIDescriptor struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	RequiresAPIKey  bool   `json:"requiresApiKey"`
	RequiresBaseURL bool   `json:"requiresBaseUrl"`
	AutoListModels  bool   `json:"autoListModels"`
	ListsModelsLive bool   `json:"listsModelsLive"`
	Doc             string `json:"doc,omitempty"`
}

// IsCustomEndpoint returns true for custom endpoint variants (*_compatible).
func IsCustomEndpoint(id string) bool {
	return strings.HasSuffix(id, "_compatible")
}

func labelForID(module kernel.ProviderModule, id string) string {
	if id == module.ID() {
		return module.Label()
	}
	if IsCustomEndpoint(id) {
		return fmt.Sprintf("%s Compatible", module.Label())
	}
	return module.Label()
}

func listingRequiresUserBaseURL(listing *kernel.ModelListing) bool {
	return listing != nil &&
		listing.Kind == kernel.ListingHTTP &&
		listing.RequiresBaseURL &&
		listing.DefaultBaseURL == ""
}

func listingIsAutoListable(listing *kernel.ModelListing) bool {
	if listing == nil {
		return false
	}
	if listing.Kind == kernel.ListingStatic {
		return true
	}
	if listing.Kind == kernel.ListingHTTP {
		return !listing.RequiresBaseURL || listing.DefaultBaseURL != ""
	}
	return false // ListingManual
}

// DescribeProviderID derives the UI descriptor for one connectable ID (module ID or alias).
func DescribeProviderID(module kernel.ProviderModule, id string) ProviderUIDescriptor {
	var listing *kernel.ModelListing
	if module != nil {
		listing = module.ModelListing(id)
	}
	custom := IsCustomEndpoint(id)

	requiresField := func(key string) bool {
		if module == nil {
			return false
		}
		for _, f := range module.UIFields() {
			if f.Key == key && f.Required {
				return true
			}
		}
		return false
	}

	requiresAPIKey := requiresField("apiKey")
	requiresBaseURL := custom || listingRequiresUserBaseURL(listing) || requiresField("baseURL")
	autoListModels := !custom && listingIsAutoListable(listing)
	listsModelsLive := !custom && listing != nil && listing.Kind == kernel.ListingHTTP && (!listing.RequiresBaseURL || listing.DefaultBaseURL != "")

	var doc string
	var label string
	if module != nil {
		doc = module.Doc()
		label = labelForID(module, id)
	}

	return ProviderUIDescriptor{
		ID:              id,
		Label:           label,
		RequiresAPIKey:  requiresAPIKey,
		RequiresBaseURL: requiresBaseURL,
		AutoListModels:  autoListModels,
		ListsModelsLive: listsModelsLive,
		Doc:             doc,
	}
}

// DescribeAllProviderIDs generates descriptors for all connectable IDs across modules.
func DescribeAllProviderIDs(modules []kernel.ProviderModule) []ProviderUIDescriptor {
	var out []ProviderUIDescriptor
	for _, m := range modules {
		if m == nil {
			continue
		}
		ids := append([]string{m.ID()}, m.Aliases()...)
		for _, id := range ids {
			out = append(out, DescribeProviderID(m, id))
		}
	}
	return out
}
