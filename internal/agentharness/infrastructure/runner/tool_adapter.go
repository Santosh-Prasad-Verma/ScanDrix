// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package runner

import (
	"encoding/json"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ToKernelToolDefinition converts a contracts.AgentTool to a kernel.ToolDefinition.
func ToKernelToolDefinition(tool contracts.AgentTool) kernel.ToolDefinition {
	schema := tool.InputSchema()

	// Convert contracts.JSONSchema to map[string]any for kernel.ToolDefinition
	var schemaMap map[string]any
	if b, err := json.Marshal(schema); err == nil {
		_ = json.Unmarshal(b, &schemaMap)
	}
	if schemaMap == nil {
		schemaMap = map[string]any{
			"type": "object",
		}
	}

	return kernel.ToolDefinition{
		Name:        tool.Name(),
		Description: tool.Description(),
		Parameters:  schemaMap,
	}
}

// ToKernelToolDefinitions converts a slice of AgentTools into kernel ToolDefinitions.
func ToKernelToolDefinitions(tools []contracts.AgentTool) []kernel.ToolDefinition {
	out := make([]kernel.ToolDefinition, len(tools))
	for i, t := range tools {
		out[i] = ToKernelToolDefinition(t)
	}
	return out
}
