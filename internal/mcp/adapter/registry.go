// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MCPRegistry coordinates multiple upstream MCP client connections,
// indexing tools across servers, handling failures resiliently, and routing calls.
type MCPRegistry struct {
	mu             sync.RWMutex
	clients        map[string]*SpecCompliantMCPClient
	configs        map[string]MCPServerConfig
	pending        map[string]chan struct{}
	toolIndex      map[string]map[string]bool // toolName -> set of serverNames
	defaultTimeout time.Duration
	maxRetries     int
	onToolsChanged func(serverName string)
}

// NewMCPRegistry initializes a new multi-server registry.
func NewMCPRegistry(opts ...MCPRegistryOptions) *MCPRegistry {
	timeout := 30 * time.Second
	retries := 3
	var toolsChangedCb func(serverName string)

	if len(opts) > 0 {
		if opts[0].DefaultTimeout > 0 {
			timeout = opts[0].DefaultTimeout
		}
		if opts[0].MaxRetries > 0 {
			retries = opts[0].MaxRetries
		}
		toolsChangedCb = opts[0].OnToolsChanged
	}

	return &MCPRegistry{
		clients:        make(map[string]*SpecCompliantMCPClient),
		configs:        make(map[string]MCPServerConfig),
		pending:        make(map[string]chan struct{}),
		toolIndex:      make(map[string]map[string]bool),
		defaultTimeout: timeout,
		maxRetries:     retries,
		onToolsChanged: toolsChangedCb,
	}
}

// Register connects and registers a remote MCP server, deduplicating concurrent attempts.
func (r *MCPRegistry) Register(ctx context.Context, config MCPServerConfig) error {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return errors.New("server config name cannot be empty")
	}

	r.mu.Lock()
	// Check if already in-flight
	if waitChan, isPending := r.pending[name]; isPending {
		r.mu.Unlock()
		select {
		case <-waitChan:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	doneChan := make(chan struct{})
	r.pending[name] = doneChan
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.pending, name)
		close(doneChan)
		r.mu.Unlock()
	}()

	transportType := config.Type
	if transportType == "" {
		transportType = TransportHTTP
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = r.defaultTimeout
	}

	retries := config.Retries
	if retries <= 0 {
		retries = r.maxRetries
	}

	var clientConfig MCPClientConfig
	clientConfig.ClientInfo.Name = fmt.Sprintf("scandrix-registry-client-%s", name)
	clientConfig.ClientInfo.Version = "1.0.0"
	clientConfig.Transport.Type = transportType
	clientConfig.Transport.URL = config.URL
	clientConfig.Transport.Headers = config.Headers
	clientConfig.Transport.Command = config.Command
	clientConfig.Transport.Args = config.Args
	clientConfig.Transport.Env = config.Env
	clientConfig.Transport.Cwd = config.Cwd
	clientConfig.Transport.Timeout = timeout
	clientConfig.Transport.Retries = retries
	clientConfig.AllowedTools = config.AllowedTools

	client := NewSpecCompliantMCPClient(clientConfig)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to register MCP server '%s': %w", name, err)
	}

	r.mu.Lock()
	r.clients[name] = client
	r.configs[name] = config
	r.markToolsDirtyLocked(name)
	r.mu.Unlock()

	return nil
}

// Unregister disconnects and unregisters a server.
func (r *MCPRegistry) Unregister(ctx context.Context, serverName string) error {
	r.mu.Lock()
	client, exists := r.clients[serverName]
	if !exists {
		r.mu.Unlock()
		return nil
	}

	delete(r.clients, serverName)
	delete(r.configs, serverName)
	r.removeServerFromIndexLocked(serverName)
	r.markToolsDirtyLocked(serverName)
	r.mu.Unlock()

	return client.Disconnect(ctx)
}

// ListAllTools aggregates all tools across connected servers, resiliently skipping failed servers.
func (r *MCPRegistry) ListAllTools(ctx context.Context) ([]MCPToolRawWithServer, error) {
	r.mu.RLock()
	clientsSnapshot := make(map[string]*SpecCompliantMCPClient, len(r.clients))
	for k, v := range r.clients {
		clientsSnapshot[k] = v
	}
	r.mu.RUnlock()

	allTools := make([]MCPToolRawWithServer, 0)
	refreshedIndex := make(map[string]map[string]bool)

	for serverName, client := range clientsSnapshot {
		// Attempt reconnect if disconnected
		if !client.IsConnected() {
			if err := client.Connect(ctx); err != nil {
				// Resilient failure: skip broken server, do not crash entire registry
				continue
			}
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			// Resilient failure: skip server
			continue
		}

		for _, tool := range tools {
			if tool == nil || tool.Name == "" {
				continue
			}

			if refreshedIndex[tool.Name] == nil {
				refreshedIndex[tool.Name] = make(map[string]bool)
			}
			refreshedIndex[tool.Name][serverName] = true

			allTools = append(allTools, MCPToolRawWithServer{
				MCPToolRaw: *tool,
				ServerName: serverName,
			})
		}
	}

	r.mu.Lock()
	r.toolIndex = refreshedIndex
	cb := r.onToolsChanged
	r.mu.Unlock()

	if cb != nil {
		cb("all")
	}

	return allTools, nil
}

// ExecuteTool routes execution to the specified server, or automatically resolves candidates from the index.
func (r *MCPRegistry) ExecuteTool(ctx context.Context, toolName string, args map[string]any, serverName ...string) (any, error) {
	if len(serverName) > 0 && serverName[0] != "" {
		client := r.resolveClientByAlias(serverName[0])
		if client == nil {
			return nil, fmt.Errorf("MCP server '%s' not found", serverName[0])
		}
		return client.ExecuteTool(ctx, toolName, args)
	}

	r.mu.RLock()
	candidates := r.getCandidatesLocked(toolName)
	r.mu.RUnlock()

	if len(candidates) == 0 {
		// Refresh index once
		_, _ = r.ListAllTools(ctx)
		r.mu.RLock()
		candidates = r.getCandidatesLocked(toolName)
		r.mu.RUnlock()
	}

	if len(candidates) > 0 {
		for _, sName := range candidates {
			r.mu.RLock()
			client := r.clients[sName]
			r.mu.RUnlock()

			if client == nil {
				continue
			}

			result, err := client.ExecuteTool(ctx, toolName, args)
			if err == nil {
				return result, nil
			}
		}
	}

	// Fallback scan across all registered clients
	r.mu.RLock()
	allClients := make([]*SpecCompliantMCPClient, 0, len(r.clients))
	for _, c := range r.clients {
		allClients = append(allClients, c)
	}
	r.mu.RUnlock()

	for _, client := range allClients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if t.Name == toolName {
				return client.ExecuteTool(ctx, toolName, args)
			}
		}
	}

	return nil, fmt.Errorf("tool '%s' not found in any registered MCP server", toolName)
}

// Destroy disconnects all clients and clears state.
func (r *MCPRegistry) Destroy(ctx context.Context) {
	r.mu.Lock()
	clients := r.clients
	r.clients = make(map[string]*SpecCompliantMCPClient)
	r.configs = make(map[string]MCPServerConfig)
	r.toolIndex = make(map[string]map[string]bool)
	cb := r.onToolsChanged
	r.mu.Unlock()

	for _, client := range clients {
		_ = client.Disconnect(ctx)
	}

	if cb != nil {
		cb("destroy")
	}
}

func (r *MCPRegistry) getCandidatesLocked(toolName string) []string {
	serversMap, exists := r.toolIndex[toolName]
	if !exists {
		return nil
	}
	var res []string
	for s := range serversMap {
		res = append(res, s)
	}
	return res
}

func (r *MCPRegistry) markToolsDirtyLocked(serverName string) {
	r.toolIndex = make(map[string]map[string]bool)
	if r.onToolsChanged != nil {
		go r.onToolsChanged(serverName)
	}
}

func (r *MCPRegistry) removeServerFromIndexLocked(serverName string) {
	for toolName, servers := range r.toolIndex {
		delete(servers, serverName)
		if len(servers) == 0 {
			delete(r.toolIndex, toolName)
		}
	}
}

func (r *MCPRegistry) resolveClientByAlias(name string) *SpecCompliantMCPClient {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if direct, exists := r.clients[name]; exists {
		return direct
	}

	targetNorm := NormalizeProviderKey(name)
	if targetNorm == "" {
		return nil
	}

	for candidateName, client := range r.clients {
		if NormalizeProviderKey(candidateName) == targetNorm {
			return client
		}
	}

	return nil
}
