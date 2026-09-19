// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"
)

// ToolContext provides run-scoped execution context to every tool.
// Domains pass services/handles via the typed Context or Services map.
type ToolContext struct {
	RunID    string
	Context  context.Context
	Services map[string]any
}

// ToolResult represents the output of a tool execution.
// Errors are values (IsError=true), never uncaught panics, allowing the
// model to recover within the loop.
type ToolResult struct {
	Output  string         `json:"output"`
	IsError bool           `json:"is_error,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// AgentTool represents a single, self-contained capability an agent can invoke.
type AgentTool interface {
	// Name returns the unique, stable identifier the model invokes.
	Name() string
	// Description provides a concise, action-oriented explanation for the model.
	Description() string
	// InputSchema returns the JSON Schema for tool arguments.
	InputSchema() JSONSchema
	// Strict returns true if strict structured tool calling is enforced for this tool.
	Strict() bool
	// Execute performs the capability given validated input and run context.
	Execute(ctx ToolContext, input any) (ToolResult, error)
}

// ToolRegistry holds a named set of tools available to an agent role.
type ToolRegistry interface {
	// Get retrieves a tool by name.
	Get(name string) (AgentTool, bool)
	// List returns all registered tools.
	List() []AgentTool
}
