// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package tools

import (
	"context"
)

// ToolHandlerFunc executes an MCP tool given input arguments.
type ToolHandlerFunc func(ctx context.Context, args map[string]any) (any, error)

// MCPTool defines a registered tool exposed over the Model Context Protocol.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema map[string]any  `json:"inputSchema"`
	Handler     ToolHandlerFunc `json:"-"`
}
