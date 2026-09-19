// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"regexp"
	"strings"
	"sync"
)

var nonAlphaNumericRegex = regexp.MustCompile(`[^a-z0-9]`)

// NormalizeProviderKey trims, lowercases, removes non-alphanumeric characters,
// and strips a trailing "mcp" suffix if length > 3.
func NormalizeProviderKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	normalized := strings.ToLower(trimmed)
	normalized = nonAlphaNumericRegex.ReplaceAllString(normalized, "")

	if normalized == "" {
		return ""
	}

	if strings.HasSuffix(normalized, "mcp") && len(normalized) > 3 {
		return normalized[:len(normalized)-3]
	}

	return normalized
}

// NormalizeToolKey trims, lowercases, and removes non-alphanumeric characters.
func NormalizeToolKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	normalized := strings.ToLower(trimmed)
	normalized = nonAlphaNumericRegex.ReplaceAllString(normalized, "")

	return normalized
}

// ThreadSafeAliasMap provides concurrency-safe access to string alias mappings.
type ThreadSafeAliasMap struct {
	mu     sync.RWMutex
	lookup map[string]string
}

// NewThreadSafeAliasMap creates a new thread-safe alias map.
func NewThreadSafeAliasMap() *ThreadSafeAliasMap {
	return &ThreadSafeAliasMap{
		lookup: make(map[string]string),
	}
}

func (m *ThreadSafeAliasMap) Set(alias, canonical string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookup[alias] = canonical
}

func (m *ThreadSafeAliasMap) Get(alias string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.lookup[alias]
	return val, ok
}

// RegisterProviderAliases registers canonical provider keys and all aliases.
func RegisterProviderAliases(aliasMap map[string]string, canonicalProvider string, aliases []string) {
	trimmedCanonical := strings.TrimSpace(canonicalProvider)
	if trimmedCanonical == "" {
		return
	}

	if _, exists := aliasMap[trimmedCanonical]; !exists {
		aliasMap[trimmedCanonical] = trimmedCanonical
	}

	normalizedCanonical := NormalizeProviderKey(trimmedCanonical)
	if normalizedCanonical != "" {
		if _, exists := aliasMap[normalizedCanonical]; !exists {
			aliasMap[normalizedCanonical] = trimmedCanonical
		}
	}

	for _, candidate := range aliases {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}

		if _, exists := aliasMap[trimmed]; !exists {
			aliasMap[trimmed] = trimmedCanonical
		}

		normalized := NormalizeProviderKey(trimmed)
		if normalized != "" {
			if _, exists := aliasMap[normalized]; !exists {
				aliasMap[normalized] = trimmedCanonical
			}
		}
	}
}

// RegisterToolAliases registers tool aliases for a given provider, including casing and normalization.
func RegisterToolAliases(toolAliases map[string]map[string]string, canonicalProvider string, toolName string) {
	trimmedProvider := strings.TrimSpace(canonicalProvider)
	trimmedTool := strings.TrimSpace(toolName)

	if trimmedProvider == "" || trimmedTool == "" {
		return
	}

	providerKey := NormalizeProviderKey(trimmedProvider)
	if providerKey == "" {
		providerKey = trimmedProvider
	}

	aliasMap, exists := toolAliases[providerKey]
	if !exists {
		aliasMap = make(map[string]string)
		toolAliases[providerKey] = aliasMap
	}

	variants := map[string]struct{}{
		trimmedTool:              {},
		strings.ToLower(trimmedTool): {},
		strings.ToUpper(trimmedTool): {},
	}

	normalizedTool := NormalizeToolKey(trimmedTool)
	if normalizedTool != "" {
		variants[normalizedTool] = struct{}{}
	}

	for alias := range variants {
		if _, ok := aliasMap[alias]; !ok {
			aliasMap[alias] = trimmedTool
		}
	}
}

// ResolveCanonicalProvider resolves a provider key against the alias map.
func ResolveCanonicalProvider(aliasMap map[string]string, provider string) string {
	trimmed := strings.TrimSpace(provider)
	if trimmed == "" {
		return ""
	}

	if direct, ok := aliasMap[trimmed]; ok {
		return direct
	}

	normalized := NormalizeProviderKey(trimmed)
	if normalized != "" {
		if resolved, ok := aliasMap[normalized]; ok {
			return resolved
		}
	}

	return trimmed
}

// ResolveCanonicalTool resolves a tool name for a given provider against known aliases.
func ResolveCanonicalTool(toolAliases map[string]map[string]string, providerAliases map[string]string, provider string, toolName string) string {
	trimmedTool := strings.TrimSpace(toolName)
	if trimmedTool == "" {
		return ""
	}

	canonicalProvider := ResolveCanonicalProvider(providerAliases, provider)
	if canonicalProvider == "" {
		canonicalProvider = provider
	}

	providerKey := NormalizeProviderKey(canonicalProvider)
	if providerKey == "" {
		providerKey = canonicalProvider
	}

	aliasMap, ok := toolAliases[providerKey]
	if !ok {
		return trimmedTool
	}

	if direct, exists := aliasMap[trimmedTool]; exists {
		return direct
	}

	if lower, exists := aliasMap[strings.ToLower(trimmedTool)]; exists {
		return lower
	}

	if upper, exists := aliasMap[strings.ToUpper(trimmedTool)]; exists {
		return upper
	}

	normalizedTool := NormalizeToolKey(trimmedTool)
	if normalizedTool != "" {
		if resolved, exists := aliasMap[normalizedTool]; exists {
			return resolved
		}
	}

	return trimmedTool
}

// MarkProviderHasMetadata records that a provider has metadata registered.
func MarkProviderHasMetadata(providersWithMetadata map[string]bool, provider string) {
	trimmed := strings.TrimSpace(provider)
	if trimmed == "" {
		return
	}

	providersWithMetadata[trimmed] = true

	normalized := NormalizeProviderKey(trimmed)
	if normalized != "" {
		providersWithMetadata[normalized] = true
	}
}
