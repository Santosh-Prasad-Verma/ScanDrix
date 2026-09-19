// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/mcp/manager/client"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPToolAdapter_Execution(t *testing.T) {
	// Mock MCP JSON-RPC Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req client.JSONRPCRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "tools/call", req.Method)
		args, ok := req.Params.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "jira_get_issue", args["name"])

		resp := client.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"content":[{"type":"text","text":"Issue summary: Fix authentication bypass"}]}`),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mcpClient := client.NewMCPClient(server.URL, nil, "jira", models.ProviderCustom)

	toolDef := models.MCPTool{
		Name:        "jira_get_issue",
		Description: "Fetch Jira ticket by key",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"issue_key": map[string]any{"type": "string"},
			},
			"required": []any{"issue_key"},
		},
	}

	adapter := AdaptMCPTool(mcpClient, toolDef)
	assert.Equal(t, "jira_get_issue", adapter.Name())
	assert.Equal(t, "Fetch Jira ticket by key", adapter.Description())
	assert.Equal(t, "object", adapter.InputSchema().Type)

	toolCtx := contracts.ToolContext{
		RunID:   "test-run",
		Context: context.Background(),
	}

	result, err := adapter.Execute(toolCtx, map[string]any{"issue_key": "PROJ-101"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Output, "Issue summary: Fix authentication bypass")
}

func TestMCPToolAdapter_ErrorHandling(t *testing.T) {
	// Server returns HTTP 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	mcpClient := client.NewMCPClient(server.URL, nil, "linear", models.ProviderCustom)
	toolDef := models.MCPTool{Name: "linear_get_issue"}
	adapter := AdaptMCPTool(mcpClient, toolDef)

	toolCtx := contracts.ToolContext{RunID: "test-err", Context: context.Background()}
	result, err := adapter.Execute(toolCtx, map[string]any{"id": "LIN-42"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Output, "MCP execution error")
}
