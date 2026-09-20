// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package providers

import (
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// IsCuratedCatalogProvider determines whether a provider's model catalog is curated/non-exhaustive
// rather than a guaranteed-complete live enumeration.
func IsCuratedCatalogProvider(providerID string) bool {
	m, ok := kernel.Get(providerID)
	if !ok {
		return false
	}
	listing := m.ModelListing(providerID)
	if listing == nil {
		return false
	}

	if listing.Kind == kernel.ListingStatic {
		return true
	}
	if listing.Kind == kernel.ListingHTTP && len(listing.FallbackModels) > 0 {
		return true
	}
	return false
}
