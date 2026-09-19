// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package providers

import (
	"fmt"
	"strings"
	"sync"
)

// ProviderFactory manages registered MCP provider instances.
type ProviderFactory struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewProviderFactory instantiates an empty registry.
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider by identifier key.
func (f *ProviderFactory) Register(key string, p Provider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.providers[strings.ToLower(key)] = p
}

// GetProvider retrieves a provider, supporting "scandrixmcp", "custom", etc.
func (f *ProviderFactory) GetProvider(providerType string) (Provider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	normalized := strings.ToLower(providerType)
	p, ok := f.providers[normalized]
	if !ok {
		return nil, fmt.Errorf("mcp provider '%s' not registered", providerType)
	}
	return p, nil
}

// GetProviders returns unique active providers.
func (f *ProviderFactory) GetProviders() []Provider {
	f.mu.RLock()
	defer f.mu.RUnlock()

	seen := make(map[Provider]bool)
	list := make([]Provider, 0, len(f.providers))
	for _, p := range f.providers {
		if !seen[p] {
			seen[p] = true
			list = append(list, p)
		}
	}
	return list
}
