// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package factory

import (
	"fmt"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// PlatformIntegrationFactory maintains a thread-safe registry of VCS adapters.
// Translates libs/platform/infrastructure/adapters/services/platformIntegration.factory.ts
type PlatformIntegrationFactory struct {
	mu                     sync.RWMutex
	codeManagementServices map[models.SCMProvider]contracts.ICodeManagementService
	aliasMap               map[string]models.SCMProvider
}

// NewPlatformIntegrationFactory instantiates an empty VCS integration factory.
func NewPlatformIntegrationFactory() *PlatformIntegrationFactory {
	f := &PlatformIntegrationFactory{
		codeManagementServices: make(map[models.SCMProvider]contracts.ICodeManagementService),
		aliasMap:               make(map[string]models.SCMProvider),
	}
	f.initDefaultAliases()
	return f
}

// NewDefaultPlatformIntegrationFactory creates a factory pre-populated with provided services.
func NewDefaultPlatformIntegrationFactory(services map[models.SCMProvider]contracts.ICodeManagementService) *PlatformIntegrationFactory {
	f := NewPlatformIntegrationFactory()
	for provider, svc := range services {
		f.RegisterCodeManagementService(provider, svc)
	}
	return f
}

func (f *PlatformIntegrationFactory) initDefaultAliases() {
	f.aliasMap["github"] = models.ProviderGitHub
	f.aliasMap["gh"] = models.ProviderGitHub
	f.aliasMap["gitlab"] = models.ProviderGitLab
	f.aliasMap["gl"] = models.ProviderGitLab
	f.aliasMap["bitbucket"] = models.ProviderBitbucket
	f.aliasMap["bitbucket_cloud"] = models.ProviderBitbucket
	f.aliasMap["bitbucket_server"] = models.ProviderBitbucket
	f.aliasMap["bitbucket_datacenter"] = models.ProviderBitbucket
	f.aliasMap["bb"] = models.ProviderBitbucket
	f.aliasMap["bbs"] = models.ProviderBitbucket
	f.aliasMap["azure"] = models.ProviderAzure
	f.aliasMap["azure_repos"] = models.ProviderAzure
	f.aliasMap["azure_devops"] = models.ProviderAzure
	f.aliasMap["ado"] = models.ProviderAzure
	f.aliasMap["forgejo"] = models.ProviderForgejo
	f.aliasMap["gitea"] = models.ProviderForgejo
}

// RegisterCodeManagementService associates a provider with an adapter implementation.
func (f *PlatformIntegrationFactory) RegisterCodeManagementService(
	provider models.SCMProvider,
	service contracts.ICodeManagementService,
) {
	if service == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	f.codeManagementServices[provider] = service

	// Register canonical lower-case alias
	canonical := strings.ToLower(strings.TrimSpace(string(provider)))
	f.aliasMap[canonical] = provider
}

// RegisterAlias maps an additional identifier (e.g., custom host or slug) to a provider.
func (f *PlatformIntegrationFactory) RegisterAlias(alias string, provider models.SCMProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aliasMap[strings.ToLower(strings.TrimSpace(alias))] = provider
}

// GetCodeManagementService retrieves an adapter by provider enum or string alias.
func (f *PlatformIntegrationFactory) GetCodeManagementService(provider models.SCMProvider) (contracts.ICodeManagementService, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if svc, exists := f.codeManagementServices[provider]; exists && svc != nil {
		return svc, nil
	}

	// Try resolving through alias map
	strKey := strings.ToLower(strings.TrimSpace(string(provider)))
	if resolvedProvider, ok := f.aliasMap[strKey]; ok {
		if svc, exists := f.codeManagementServices[resolvedProvider]; exists && svc != nil {
			return svc, nil
		}
	}

	return nil, fmt.Errorf("repository service for type '%s' not found", provider)
}

// GetCodeManagementServiceByString resolves an adapter using a free-form string key.
func (f *PlatformIntegrationFactory) GetCodeManagementServiceByString(rawType string) (contracts.ICodeManagementService, error) {
	if rawType == "" || rawType == "null" || rawType == "undefined" {
		return nil, fmt.Errorf("repository service for type '%s' not found", rawType)
	}

	cleanType := strings.ToLower(strings.TrimSpace(rawType))

	f.mu.RLock()
	defer f.mu.RUnlock()

	if provider, ok := f.aliasMap[cleanType]; ok {
		if svc, exists := f.codeManagementServices[provider]; exists && svc != nil {
			return svc, nil
		}
	}

	// Direct enum match attempt
	provider := models.SCMProvider(strings.ToUpper(cleanType))
	if svc, exists := f.codeManagementServices[provider]; exists && svc != nil {
		return svc, nil
	}

	return nil, fmt.Errorf("repository service for type '%s' not found", rawType)
}

// HasCodeManagementService checks if a provider is actively registered.
func (f *PlatformIntegrationFactory) HasCodeManagementService(provider models.SCMProvider) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if _, exists := f.codeManagementServices[provider]; exists {
		return true
	}
	strKey := strings.ToLower(strings.TrimSpace(string(provider)))
	if p, ok := f.aliasMap[strKey]; ok {
		_, exists := f.codeManagementServices[p]
		return exists
	}
	return false
}

// ListRegisteredProviders returns all currently active SCM providers.
func (f *PlatformIntegrationFactory) ListRegisteredProviders() []models.SCMProvider {
	f.mu.RLock()
	defer f.mu.RUnlock()

	providers := make([]models.SCMProvider, 0, len(f.codeManagementServices))
	for p := range f.codeManagementServices {
		providers = append(providers, p)
	}
	return providers
}

// Unregister removes a provider from the factory.
func (f *PlatformIntegrationFactory) Unregister(provider models.SCMProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.codeManagementServices, provider)
}
