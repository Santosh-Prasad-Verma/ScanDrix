// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/mcp/manager/config"
)

// ProviderDescriptions maps app names to human-readable summaries.
type ProviderDescriptions struct {
	Integrations map[string]string `json:"integrations"`
}

// IntegrationDescriptionService dynamically provides descriptions for integrations.
type IntegrationDescriptionService struct {
	mu           sync.RWMutex
	descriptions map[string]ProviderDescriptions
}

var (
	defaultServiceInstance *IntegrationDescriptionService
	once                    sync.Once
)

// GetIntegrationDescriptionService returns the singleton description service.
func GetIntegrationDescriptionService() *IntegrationDescriptionService {
	once.Do(func() {
		defaultServiceInstance = NewIntegrationDescriptionService()
	})
	return defaultServiceInstance
}

// NewIntegrationDescriptionService initializes the service by parsing bundled descriptions JSON.
func NewIntegrationDescriptionService() *IntegrationDescriptionService {
	s := &IntegrationDescriptionService{
		descriptions: make(map[string]ProviderDescriptions),
	}

	if len(config.EmbeddedIntegrationDescriptions) > 0 {
		var raw map[string]ProviderDescriptions
		if err := json.Unmarshal(config.EmbeddedIntegrationDescriptions, &raw); err == nil {
			s.descriptions = raw
		}
	}

	return s
}

// GetDescription returns the curated description or generates a sensible fallback.
func (s *IntegrationDescriptionService) GetDescription(provider, appName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if provMap, ok := s.descriptions[provider]; ok {
		if desc, ok := provMap.Integrations[appName]; ok && desc != "" {
			return desc
		}
	}

	return s.generateFallbackDescription(appName)
}

func (s *IntegrationDescriptionService) generateFallbackDescription(appName string) string {
	if appName == "" {
		return "Integration with third-party service for automated code review context."
	}
	formatted := strings.ToUpper(appName[:1]) + appName[1:]
	return fmt.Sprintf("Integration with %s for automation and task management.", formatted)
}
