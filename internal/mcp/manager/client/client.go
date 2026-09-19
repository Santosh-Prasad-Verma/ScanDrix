// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// MCPClient communicates with remote Model Context Protocol servers over HTTP JSON-RPC 2.0.
type MCPClient struct {
	httpClient   *http.Client
	baseURL      string
	headers      map[string]string
	serverName   string
	providerType models.MCPProviderType
}

// NewMCPClient creates a client configured with circuit-breaker timeouts.
func NewMCPClient(baseURL string, headers map[string]string, serverName string, providerType models.MCPProviderType) *MCPClient {
	if headers == nil {
		headers = make(map[string]string)
	}
	return &MCPClient{
		httpClient: &http.Client{
			Timeout: 8 * time.Second, // 8-second circuit breaker deadline
		},
		baseURL:      baseURL,
		headers:      headers,
		serverName:   serverName,
		providerType: providerType,
	}
}

// JSONRPCRequest represents an outgoing MCP request.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// JSONRPCResponse represents an incoming MCP response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   any             `json:"error,omitempty"`
}

// RawToolDefinition represents the tool schema returned by an MCP server.
type RawToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations struct {
		ReadOnlyHint bool `json:"readOnlyHint"`
	} `json:"annotations"`
}

// GetTools sends a "tools/list" JSON-RPC request to the remote MCP server.
func (c *MCPClient) GetTools(ctx context.Context) ([]models.MCPTool, error) {
	reqPayload := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]any{},
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote MCP request to %s failed: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remote MCP server error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var jsonRPCResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonRPCResp); err != nil {
		return nil, fmt.Errorf("failed decoding JSON-RPC response: %w", err)
	}

	if jsonRPCResp.Error != nil {
		errBytes, _ := json.Marshal(jsonRPCResp.Error)
		return nil, fmt.Errorf("remote MCP tool error: %s", string(errBytes))
	}

	var result struct {
		Tools []RawToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(jsonRPCResp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed unmarshaling tools list: %w", err)
	}

	tools := make([]models.MCPTool, 0, len(result.Tools))
	for _, t := range result.Tools {
		tools = append(tools, models.MCPTool{
			Slug:        t.Name,
			Name:        t.Name,
			Description: t.Description,
			Provider:    c.providerType,
			ReadOnly:    t.Annotations.ReadOnlyHint,
			InputSchema: t.InputSchema,
		})
	}

	return tools, nil
}

// CallTool executes a specific tool on the remote MCP server.
func (c *MCPClient) CallTool(ctx context.Context, toolName string, arguments map[string]any) (string, error) {
	reqPayload := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("tool call error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var jsonRPCResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonRPCResp); err != nil {
		return "", err
	}

	if jsonRPCResp.Error != nil {
		errBytes, _ := json.Marshal(jsonRPCResp.Error)
		return "", errors.New(string(errBytes))
	}

	return string(jsonRPCResp.Result), nil
}
