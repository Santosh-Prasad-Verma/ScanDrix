package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
)

// MCPClient connects to an MCP server to discover tools and execute agent actions.
type MCPClient struct {
	server *MCPServer
	seqID  int64
}

// NewInProcessClient connects directly to an in-process MCP server.
func NewInProcessClient(server *MCPServer) *MCPClient {
	return &MCPClient{
		server: server,
	}
}

// Initialize performs handshake and capabilities exchange.
func (c *MCPClient) Initialize(ctx context.Context) (map[string]any, error) {
	id := atomic.AddInt64(&c.seqID, 1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "initialize",
	}

	resp := c.server.HandleRequest(ctx, req)
	if resp.Error != nil {
		return nil, fmt.Errorf("initialize failed: %s", resp.Error.Message)
	}

	resMap, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, errors.New("invalid initialize response payload")
	}
	return resMap, nil
}

// ListTools discovers available agent tools.
func (c *MCPClient) ListTools(ctx context.Context) ([]Tool, error) {
	id := atomic.AddInt64(&c.seqID, 1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/list",
	}

	resp := c.server.HandleRequest(ctx, req)
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list failed: %s", resp.Error.Message)
	}

	resMap, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, errors.New("invalid tools/list response format")
	}

	rawTools, ok := resMap["tools"].([]Tool)
	if !ok {
		// Attempt JSON conversion if deserialized as generic []any
		bytes, _ := json.Marshal(resMap["tools"])
		var tools []Tool
		if err := json.Unmarshal(bytes, &tools); err != nil {
			return nil, err
		}
		return tools, nil
	}

	return rawTools, nil
}

// CallTool invokes a tool by name with arguments.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolCallResult, error) {
	id := atomic.AddInt64(&c.seqID, 1)
	paramsBytes, err := json.Marshal(map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params:  paramsBytes,
	}

	resp := c.server.HandleRequest(ctx, req)
	if resp.Error != nil {
		return nil, fmt.Errorf("tool call error: %s", resp.Error.Message)
	}

	if resultPtr, ok := resp.Result.(*ToolCallResult); ok {
		return resultPtr, nil
	}
	if resultVal, ok := resp.Result.(ToolCallResult); ok {
		return &resultVal, nil
	}

	// Fallback conversion
	bytes, _ := json.Marshal(resp.Result)
	var res ToolCallResult
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ReadResource fetches resource text by URI.
func (c *MCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	id := atomic.AddInt64(&c.seqID, 1)
	paramsBytes, err := json.Marshal(map[string]string{
		"uri": uri,
	})
	if err != nil {
		return "", err
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "resources/read",
		Params:  paramsBytes,
	}

	resp := c.server.HandleRequest(ctx, req)
	if resp.Error != nil {
		return "", fmt.Errorf("resources/read error: %s", resp.Error.Message)
	}

	resMap, ok := resp.Result.(map[string]any)
	if !ok {
		return "", errors.New("invalid resources/read result format")
	}

	contents, ok := resMap["contents"].([]map[string]string)
	if ok && len(contents) > 0 {
		return contents[0]["text"], nil
	}

	return "", nil
}
