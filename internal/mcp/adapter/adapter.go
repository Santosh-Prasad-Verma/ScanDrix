// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type defaultMCPAdapter struct {
	config         MCPAdapterConfig
	registry       *MCPRegistry
	isConnected    bool
	toolIndexDirty bool
	mu             sync.RWMutex
	ensureMu       sync.Mutex
}

// CreateMCPAdapter creates an aggregated MCP adapter coordinating multiple upstream MCP servers.
func CreateMCPAdapter(config MCPAdapterConfig) MCPAdapter {
	adapter := &defaultMCPAdapter{
		config:         config,
		toolIndexDirty: true,
	}

	adapter.registry = NewMCPRegistry(MCPRegistryOptions{
		DefaultTimeout: config.DefaultTimeout,
		MaxRetries:     config.MaxRetries,
		OnToolsChanged: func(serverName string) {
			adapter.mu.Lock()
			adapter.toolIndexDirty = true
			adapter.mu.Unlock()
		},
	})

	return adapter
}

func (a *defaultMCPAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	wasConnected := a.isConnected
	a.mu.Unlock()

	if wasConnected {
		_ = a.Disconnect(ctx)
	}

	if len(a.config.Servers) == 0 {
		a.mu.Lock()
		a.isConnected = false
		a.mu.Unlock()
		return errors.New("no MCP servers configured: unable to establish connection")
	}

	var wg sync.WaitGroup
	var successCount int64
	var muCount sync.Mutex
	var errs []string

	for _, srv := range a.config.Servers {
		wg.Add(1)
		go func(s MCPServerConfig) {
			defer wg.Done()
			if err := a.registry.Register(ctx, s); err != nil {
				muCount.Lock()
				errs = append(errs, fmt.Sprintf("server '%s': %v", s.Name, err))
				muCount.Unlock()

				if a.config.OnError != nil {
					a.config.OnError(err, s.Name)
				}
			} else {
				muCount.Lock()
				successCount++
				muCount.Unlock()
			}
		}(srv)
	}

	wg.Wait()

	if successCount == 0 {
		a.mu.Lock()
		a.isConnected = false
		a.mu.Unlock()
		return fmt.Errorf("failed to connect to any MCP server: %v", errs)
	}

	a.mu.Lock()
	a.isConnected = true
	a.toolIndexDirty = false
	a.mu.Unlock()

	return nil
}

func (a *defaultMCPAdapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	if !a.isConnected {
		a.mu.Unlock()
		return nil
	}
	a.isConnected = false
	a.toolIndexDirty = true
	a.mu.Unlock()

	a.registry.Destroy(ctx)
	return nil
}

func (a *defaultMCPAdapter) EnsureConnection(ctx context.Context) error {
	a.ensureMu.Lock()
	defer a.ensureMu.Unlock()

	a.mu.RLock()
	connected := a.isConnected
	dirty := a.toolIndexDirty
	a.mu.RUnlock()

	if !connected {
		if err := a.Connect(ctx); err != nil {
			return err
		}
		_, err := a.registry.ListAllTools(ctx)
		return err
	}

	if dirty {
		_, err := a.registry.ListAllTools(ctx)
		if err != nil {
			// Reconnect on failure
			_ = a.Disconnect(ctx)
			if err := a.Connect(ctx); err != nil {
				return err
			}
			_, err = a.registry.ListAllTools(ctx)
			return err
		}
		a.mu.Lock()
		a.toolIndexDirty = false
		a.mu.Unlock()
	}

	return nil
}

func (a *defaultMCPAdapter) GetTools(ctx context.Context) ([]*MCPTool, error) {
	if err := a.EnsureConnection(ctx); err != nil {
		return nil, err
	}

	rawTools, err := a.registry.ListAllTools(ctx)
	if err != nil {
		return nil, err
	}

	engineTools, err := MCPToolsToEngineTools(rawTools)
	if err != nil {
		return nil, err
	}

	mcpTools := make([]*MCPTool, 0, len(engineTools))
	for _, et := range engineTools {
		toolName := et.Name
		mcpTools = append(mcpTools, &MCPTool{
			MCPToolRaw: MCPToolRaw{
				Name:         et.Name,
				Title:        et.Title,
				Description:  et.Description,
				InputSchema:  et.InputSchema,
				OutputSchema: et.OutputSchema,
				Annotations:  et.Annotations,
			},
			Execute: func(execCtx context.Context, args map[string]any) (any, error) {
				return a.registry.ExecuteTool(execCtx, toolName, args)
			},
		})
	}

	return mcpTools, nil
}

func (a *defaultMCPAdapter) HasTool(ctx context.Context, name string) (bool, error) {
	tools, err := a.GetTools(ctx)
	if err != nil {
		return false, err
	}
	for _, t := range tools {
		if t.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (a *defaultMCPAdapter) ListResources(ctx context.Context) ([]MCPResourceWithServer, error) {
	if err := a.EnsureConnection(ctx); err != nil {
		return nil, err
	}

	a.registry.mu.RLock()
	clients := make(map[string]*SpecCompliantMCPClient, len(a.registry.clients))
	for k, v := range a.registry.clients {
		clients[k] = v
	}
	a.registry.mu.RUnlock()

	var allRes []MCPResourceWithServer
	for sName, c := range clients {
		res, err := c.ListResources(ctx)
		if err != nil {
			continue
		}
		for _, r := range res {
			allRes = append(allRes, MCPResourceWithServer{
				MCPResource: r,
				ServerName:  sName,
			})
		}
	}

	return allRes, nil
}

func (a *defaultMCPAdapter) ReadResource(ctx context.Context, uri string, serverName ...string) (any, error) {
	if err := a.EnsureConnection(ctx); err != nil {
		return nil, err
	}

	if len(serverName) > 0 && serverName[0] != "" {
		c := a.registry.resolveClientByAlias(serverName[0])
		if c == nil {
			return nil, fmt.Errorf("server '%s' not found", serverName[0])
		}
		return c.ReadResource(ctx, uri)
	}

	a.registry.mu.RLock()
	clients := make([]*SpecCompliantMCPClient, 0, len(a.registry.clients))
	for _, c := range a.registry.clients {
		clients = append(clients, c)
	}
	a.registry.mu.RUnlock()

	for _, c := range clients {
		data, err := c.ReadResource(ctx, uri)
		if err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("resource '%s' not accessible from any connected server", uri)
}

func (a *defaultMCPAdapter) ListPrompts(ctx context.Context) ([]MCPPromptWithServer, error) {
	if err := a.EnsureConnection(ctx); err != nil {
		return nil, err
	}

	a.registry.mu.RLock()
	clients := make(map[string]*SpecCompliantMCPClient, len(a.registry.clients))
	for k, v := range a.registry.clients {
		clients[k] = v
	}
	a.registry.mu.RUnlock()

	var allPrompts []MCPPromptWithServer
	for sName, c := range clients {
		prompts, err := c.ListPrompts(ctx)
		if err != nil {
			continue
		}
		for _, p := range prompts {
			allPrompts = append(allPrompts, MCPPromptWithServer{
				MCPPrompt:  p,
				ServerName: sName,
			})
		}
	}

	return allPrompts, nil
}

func (a *defaultMCPAdapter) GetPrompt(ctx context.Context, name string, args map[string]string, serverName ...string) (any, error) {
	if err := a.EnsureConnection(ctx); err != nil {
		return nil, err
	}

	if len(serverName) > 0 && serverName[0] != "" {
		c := a.registry.resolveClientByAlias(serverName[0])
		if c == nil {
			return nil, fmt.Errorf("server '%s' not found", serverName[0])
		}
		return c.GetPrompt(ctx, name, args)
	}

	a.registry.mu.RLock()
	clients := make([]*SpecCompliantMCPClient, 0, len(a.registry.clients))
	for _, c := range a.registry.clients {
		clients = append(clients, c)
	}
	a.registry.mu.RUnlock()

	for _, c := range clients {
		res, err := c.GetPrompt(ctx, name, args)
		if err == nil {
			return res, nil
		}
	}

	return nil, fmt.Errorf("prompt '%s' not found in any registered MCP server", name)
}

func (a *defaultMCPAdapter) ExecuteTool(ctx context.Context, name string, args map[string]any, serverName ...string) (any, error) {
	if err := a.EnsureConnection(ctx); err != nil {
		return nil, err
	}
	return a.registry.ExecuteTool(ctx, name, args, serverName...)
}

func (a *defaultMCPAdapter) GetMetrics() map[string]any {
	a.registry.mu.RLock()
	defer a.registry.mu.RUnlock()

	metrics := make(map[string]any)
	metrics["totalClients"] = len(a.registry.clients)
	return metrics
}

func (a *defaultMCPAdapter) GetRegistry() any {
	return a.registry
}
