// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"context"
	"strings"
	"time"
)

// MCPToolMetadata captures argument requirements and schema definitions for tools.
type MCPToolMetadata struct {
	RequiredArgs []string       `json:"requiredArgs"`
	InputSchema  map[string]any `json:"inputSchema,omitempty"`
}

// MetadataLoadResult groups filtered connection configs and the resolved metadata dictionary.
type MetadataLoadResult struct {
	Connections []MCPServerConfig          `json:"connections"`
	Metadata    map[string]*MCPToolMetadata `json:"metadata"`
}

// ResolvedToolMetadata contains resolved provider and tool identities along with metadata.
type ResolvedToolMetadata struct {
	ProviderID string           `json:"providerId"`
	ToolName   string           `json:"toolName"`
	Metadata   *MCPToolMetadata `json:"metadata"`
}

// MCPToolMetadataService coordinates tool metadata discovery and caching for organizations.
type MCPToolMetadataService struct{}

// NewMCPToolMetadataService initializes the metadata service.
func NewMCPToolMetadataService() *MCPToolMetadataService {
	return &MCPToolMetadataService{}
}

// LoadMetadataFromConnections queries tools across connections and constructs the normalized metadata map.
func (s *MCPToolMetadataService) LoadMetadataFromConnections(ctx context.Context, connections []MCPServerConfig) (*MetadataLoadResult, error) {
	if len(connections) == 0 {
		return &MetadataLoadResult{
			Connections: []MCPServerConfig{},
			Metadata:    make(map[string]*MCPToolMetadata),
		}, nil
	}

	metadataMap := make(map[string]*MCPToolMetadata)
	providersWithMetadata := make(map[string]bool)

	adapter := CreateMCPAdapter(MCPAdapterConfig{
		Servers:        connections,
		DefaultTimeout: 60 * time.Second,
		MaxRetries:     1,
	})

	if err := adapter.Connect(ctx); err == nil {
		defer func() { _ = adapter.Disconnect(ctx) }()

		reg, ok := adapter.GetRegistry().(*MCPRegistry)
		if ok && reg != nil {
			tools, _ := reg.ListAllTools(ctx)

			providerIndex := make(map[string]string)
			for _, conn := range connections {
				providerID := s.resolveConnectionProviderID(conn)
				if providerID == "" {
					continue
				}

				aliases := []string{conn.Name, conn.Provider, conn.URL}
				for _, alias := range aliases {
					trimmed := strings.TrimSpace(alias)
					if trimmed == "" {
						continue
					}
					providerIndex[trimmed] = providerID
					providerIndex[strings.ToLower(trimmed)] = providerID
				}
			}

			for _, tool := range tools {
				trimmedServer := strings.TrimSpace(tool.ServerName)
				providerID, exists := providerIndex[trimmedServer]
				if !exists {
					providerID = providerIndex[strings.ToLower(trimmedServer)]
				}
				if providerID == "" {
					continue
				}

				requiredArgs := s.extractRequiredArgs(tool.InputSchema)
				meta := &MCPToolMetadata{
					RequiredArgs: requiredArgs,
					InputSchema:  tool.InputSchema,
				}

				key := providerID + "|" + tool.Name
				metadataMap[key] = meta
				MarkProviderHasMetadata(providersWithMetadata, providerID)
			}
		}
	}

	// Filter connections that successfully resolved metadata
	var filteredConnections []MCPServerConfig
	for _, conn := range connections {
		canonical := strings.TrimSpace(conn.Provider)
		if canonical == "" {
			canonical = strings.TrimSpace(conn.Name)
		}
		if canonical == "" {
			canonical = strings.TrimSpace(conn.URL)
		}
		if canonical == "" {
			continue
		}

		if providersWithMetadata[canonical] {
			filteredConnections = append(filteredConnections, conn)
			continue
		}

		norm := NormalizeProviderKey(canonical)
		if norm != "" && providersWithMetadata[norm] {
			filteredConnections = append(filteredConnections, conn)
		}
	}

	return &MetadataLoadResult{
		Connections: filteredConnections,
		Metadata:    metadataMap,
	}, nil
}

// GetMetadataForTool retrieves metadata for a specific provider and tool.
func (s *MCPToolMetadataService) GetMetadataForTool(metadataMap map[string]*MCPToolMetadata, providerID, toolName string) *MCPToolMetadata {
	res, ok := s.ResolveToolMetadata(metadataMap, providerID, toolName)
	if !ok {
		return nil
	}
	return res.Metadata
}

// ResolveToolMetadata resolves metadata using direct and normalized fallback matching.
func (s *MCPToolMetadataService) ResolveToolMetadata(metadataMap map[string]*MCPToolMetadata, providerID, toolName string) (*ResolvedToolMetadata, bool) {
	trimmedProvider := strings.TrimSpace(providerID)
	trimmedTool := strings.TrimSpace(toolName)

	if trimmedProvider == "" || trimmedTool == "" || metadataMap == nil {
		return nil, false
	}

	// 1. Direct match
	directKey := trimmedProvider + "|" + trimmedTool
	if direct, exists := metadataMap[directKey]; exists {
		return &ResolvedToolMetadata{
			ProviderID: trimmedProvider,
			ToolName:   trimmedTool,
			Metadata:   direct,
		}, true
	}

	// 2. Normalized matching
	normProvider := NormalizeProviderKey(trimmedProvider)
	normTool := NormalizeToolKey(trimmedTool)

	for key, meta := range metadataMap {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		candidateProvider, candidateTool := parts[0], parts[1]

		if s.providersMatch(candidateProvider, trimmedProvider, normProvider) &&
			s.toolsMatch(candidateTool, trimmedTool, normTool) {
			return &ResolvedToolMetadata{
				ProviderID: candidateProvider,
				ToolName:   candidateTool,
				Metadata:   meta,
			}, true
		}
	}

	return nil, false
}

func (s *MCPToolMetadataService) resolveConnectionProviderID(conn MCPServerConfig) string {
	candidates := []string{conn.Provider, conn.Name, conn.URL}
	for _, c := range candidates {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *MCPToolMetadataService) extractRequiredArgs(schema map[string]any) []string {
	if schema == nil {
		return []string{}
	}

	var required []string
	if rawReq, ok := schema["required"].([]any); ok {
		for _, item := range rawReq {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				required = append(required, strings.TrimSpace(str))
			}
		}
	} else if strReq, ok := schema["required"].([]string); ok {
		for _, str := range strReq {
			if strings.TrimSpace(str) != "" {
				required = append(required, strings.TrimSpace(str))
			}
		}
	}

	if len(required) > 0 {
		return required
	}

	// Infer required from properties with required: true
	if props, ok := schema["properties"].(map[string]any); ok {
		for k, v := range props {
			if propMap, isMap := v.(map[string]any); isMap {
				if reqFlag, hasFlag := propMap["required"].(bool); hasFlag && reqFlag {
					required = append(required, k)
				}
			}
		}
	}

	return required
}

func (s *MCPToolMetadataService) providersMatch(candidate, requested, requestedNorm string) bool {
	trimmedCandidate := strings.TrimSpace(candidate)
	if trimmedCandidate == "" {
		return false
	}
	if trimmedCandidate == requested {
		return true
	}

	candidateNorm := NormalizeProviderKey(trimmedCandidate)
	if candidateNorm == "" {
		return false
	}

	if candidateNorm == requestedNorm {
		return true
	}

	requestedFallback := NormalizeProviderKey(requested)
	return requestedFallback != "" && candidateNorm == requestedFallback
}

func (s *MCPToolMetadataService) toolsMatch(candidate, requested, requestedNorm string) bool {
	trimmedCandidate := strings.TrimSpace(candidate)
	if trimmedCandidate == "" {
		return false
	}
	if trimmedCandidate == requested {
		return true
	}

	candidateNorm := NormalizeToolKey(trimmedCandidate)
	if candidateNorm == "" {
		return false
	}

	if candidateNorm == requestedNorm {
		return true
	}

	requestedFallback := NormalizeToolKey(requested)
	return requestedFallback != "" && candidateNorm == requestedFallback
}
