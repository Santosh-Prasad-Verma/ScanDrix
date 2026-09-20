// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package kernel

import (
	"strings"
	"sync"
)

// ProviderRegistry tracks registered provider modules by their ID and aliases.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]ProviderModule
}

// NewProviderRegistry creates an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ProviderModule),
	}
}

// Register registers a provider module under its primary ID and all aliases.
func (r *ProviderRegistry) Register(m ProviderModule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := strings.ToLower(strings.TrimSpace(m.ID()))
	r.providers[id] = m

	for _, alias := range m.Aliases() {
		aliasKey := strings.ToLower(strings.TrimSpace(alias))
		r.providers[aliasKey] = m
	}
}

// Get retrieves a provider module by ID or alias.
func (r *ProviderRegistry) Get(id string) (ProviderModule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(id))
	m, ok := r.providers[key]
	return m, ok
}

// Has checks whether a provider module exists under the ID or alias.
func (r *ProviderRegistry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

// List returns a deduplicated slice of all registered provider modules.
func (r *ProviderRegistry) List() []ProviderModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[ProviderModule]bool)
	var result []ProviderModule
	for _, m := range r.providers {
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}

// DefaultRegistry is the global singleton registry.
var DefaultRegistry = NewProviderRegistry()

// Register adds a provider to the default global registry.
func Register(m ProviderModule) {
	DefaultRegistry.Register(m)
}

// Get retrieves a provider from the default global registry.
func Get(id string) (ProviderModule, bool) {
	return DefaultRegistry.Get(id)
}

// List returns all registered providers in the default global registry.
func List() []ProviderModule {
	return DefaultRegistry.List()
}
