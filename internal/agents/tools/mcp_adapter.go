// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"encoding/json"
	"fmt"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	ahTools "github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
	"github.com/scandrix/backend/internal/mcp/manager/client"
	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// MCPToolAdapter wraps a remote MCPTool and delegates execution over JSON-RPC via MCPClient.
// It implements the domain-agnostic contracts.AgentTool port.
type MCPToolAdapter struct {
	client *client.MCPClient
	tool   models.MCPTool
	schema contracts.JSONSchema
}

// AdaptMCPTool wraps a single MCP tool definition into an AgentTool.
func AdaptMCPTool(mcpClient *client.MCPClient, tool models.MCPTool) contracts.AgentTool {
	var schema contracts.JSONSchema
	if tool.InputSchema != nil {
		if b, err := json.Marshal(tool.InputSchema); err == nil {
			_ = json.Unmarshal(b, &schema)
		}
	}
	if schema.Type == "" {
		schema = contracts.JSONSchema{Type: "object"}
	}

	return &MCPToolAdapter{
		client: mcpClient,
		tool:   tool,
		schema: schema,
	}
}

// AdaptMCPTools converts a slice of remote MCPTools into AgentTools.
func AdaptMCPTools(mcpClient *client.MCPClient, tools []models.MCPTool) []contracts.AgentTool {
	adapted := make([]contracts.AgentTool, 0, len(tools))
	for _, t := range tools {
		adapted = append(adapted, AdaptMCPTool(mcpClient, t))
	}
	return adapted
}

// BuildMCPToolRegistry packs all given remote MCP tools into an InMemoryToolRegistry.
func BuildMCPToolRegistry(mcpClient *client.MCPClient, tools []models.MCPTool) contracts.ToolRegistry {
	agentTools := AdaptMCPTools(mcpClient, tools)
	return ahTools.NewInMemoryToolRegistry(agentTools...)
}

func (a *MCPToolAdapter) Name() string {
	if a.tool.Name != "" {
		return a.tool.Name
	}
	return a.tool.Slug
}

func (a *MCPToolAdapter) Description() string {
	return a.tool.Description
}

func (a *MCPToolAdapter) InputSchema() contracts.JSONSchema {
	return a.schema
}

func (a *MCPToolAdapter) Strict() bool {
	return false
}

func (a *MCPToolAdapter) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	if a.client == nil {
		return contracts.ToolResult{
			Output:  fmt.Sprintf("MCP tool %s has no connected client", a.Name()),
			IsError: true,
		}, nil
	}

	args := make(map[string]any)
	if input != nil {
		switch v := input.(type) {
		case map[string]any:
			args = v
		case string:
			_ = json.Unmarshal([]byte(v), &args)
		default:
			if b, err := json.Marshal(v); err == nil {
				_ = json.Unmarshal(b, &args)
			}
		}
	}

	execCtx := ctx.Context
	if execCtx == nil {
		execCtx = ctx.Context
	}

	output, err := a.client.CallTool(execCtx, a.Name(), args)
	if err != nil {
		return contracts.ToolResult{
			Output:  fmt.Sprintf("MCP execution error on tool %s: %v", a.Name(), err),
			IsError: true,
		}, nil
	}

	return contracts.ToolResult{
		Output:  output,
		IsError: false,
	}, nil
}
