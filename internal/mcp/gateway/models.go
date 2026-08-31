package gateway

import (
	"encoding/json"
)

// Standard JSON-RPC 2.0 and MCP Security Error Codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeUnauthorized   = -32001
	CodeForbidden      = -32003
)

// AgentRole defines permission levels for MCP autonomous subagents.
type AgentRole string

const (
	RoleReviewer AgentRole = "reviewer" // Read-only AST analysis, linting, and rule lookup
	RoleAuditor  AgentRole = "auditor"  // Read-only CVE lookup, security catalog search
	RoleMutator  AgentRole = "mutator"  // Code comment creation, patch mutation
	RoleAdmin    AgentRole = "admin"    // Full administrative tool execution
)

// JSONRPCRequest represents a standard JSON-RPC 2.0 message.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a standard JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError provides structured error details.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Tool defines an invokable capability in the Model Context Protocol.
type Tool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	AllowedRoles []AgentRole    `json:"allowedRoles,omitempty"`
	IsReadOnly   bool           `json:"isReadOnly"`
}

// ToolContent encapsulates a piece of content returned by a tool call.
type ToolContent struct {
	Type string `json:"type"` // "text", "resource", "image"
	Text string `json:"text,omitempty"`
}

// ToolCallResult represents the outcome of calling an MCP tool.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// Resource represents a readable context asset (code file, rule catalog, schema).
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// PromptTemplate defines a reusable prompt structure.
type PromptTemplate struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument specifies an input parameter for a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
