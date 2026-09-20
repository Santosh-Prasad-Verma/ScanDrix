// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"context"
	"regexp"
	"time"
)

// SessionID, TenantID, and ThreadID are domain identifier types.
type SessionID = string
type TenantID = string
type ThreadID = string

// TransportType defines the supported MCP transport protocols.
type TransportType string

const (
	TransportHTTP      TransportType = "http"
	TransportSSE       TransportType = "sse"
	TransportWebSocket TransportType = "websocket"
	TransportStdio     TransportType = "stdio"
)

// CompleteClientCapabilities models client capabilities for protocol handshake.
type CompleteClientCapabilities struct {
	Tools       *ToolsCapability       `json:"tools,omitempty"`
	Resources   *ResourcesCapability   `json:"resources,omitempty"`
	Prompts     *PromptsCapability     `json:"prompts,omitempty"`
	Roots       *RootsCapability       `json:"roots,omitempty"`
	Sampling    map[string]any         `json:"sampling,omitempty"`
	Elicitation map[string]any         `json:"elicitation,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ResourcesCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
	Subscribe   bool `json:"subscribe,omitempty"`
}

type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// TenantContext encapsulates tenant isolation, roots, quotas, and permissions.
type TenantContext struct {
	TenantID     string   `json:"tenantId"`
	UserID       string   `json:"userId,omitempty"`
	Permissions  []string `json:"permissions"`
	AllowedRoots []string `json:"allowedRoots"`
	Quotas       Quotas   `json:"quotas"`
}

type Quotas struct {
	MaxRequests int `json:"maxRequests"`
	MaxTokens   int `json:"maxTokens"`
	RateLimit   int `json:"rateLimit"`
}

// SecurityPolicy enforces enterprise security boundaries per MCP connection.
type SecurityPolicy struct {
	AllowedURIPatterns   []*regexp.Regexp `json:"-"`
	BlockedURIPatterns   []*regexp.Regexp `json:"-"`
	MaxFileSize          int64            `json:"maxFileSize"`
	PreventPathTraversal bool             `json:"preventPathTraversal"`
	RequireHumanApproval bool             `json:"requireHumanApproval"`
}

// MCPMetrics aggregates real-time telemetry across MCP client operations.
type MCPMetrics struct {
	ConnectionsTotal      int64                     `json:"connectionsTotal"`
	ConnectionsActive     int64                     `json:"connectionsActive"`
	ConnectionErrors      int64                     `json:"connectionErrors"`
	RequestsTotal         int64                     `json:"requestsTotal"`
	RequestsSuccessful    int64                     `json:"requestsSuccessful"`
	RequestsFailed        int64                     `json:"requestsFailed"`
	RequestDuration       []float64                 `json:"requestDuration"`
	ToolCalls             int64                     `json:"toolCalls"`
	ResourceReads         int64                     `json:"resourceReads"`
	PromptGets            int64                     `json:"promptGets"`
	SamplingRequests      int64                     `json:"samplingRequests"`
	ElicitationRequests   int64                     `json:"elicitationRequests"`
	SecurityViolations    int64                     `json:"securityViolations"`
	UnauthorizedAccess    int64                     `json:"unauthorizedAccess"`
	PathTraversalAttempts int64                     `json:"pathTraversalAttempts"`
	TenantMetrics         map[string]*TenantMetric  `json:"tenantMetrics"`
}

type TenantMetric struct {
	Requests   int64 `json:"requests"`
	TokensUsed int64 `json:"tokensUsed"`
	Errors     int64 `json:"errors"`
}

// AuditEvent records high-fidelity security and lifecycle audit trails.
type AuditEvent struct {
	Timestamp int64          `json:"timestamp"`
	TenantID  string         `json:"tenantId"`
	UserID    string         `json:"userId,omitempty"`
	Event     string         `json:"event"`
	Resource  string         `json:"resource,omitempty"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// HumanApprovalRequest models human-in-the-loop oversight requests.
type HumanApprovalRequest struct {
	Type    string         `json:"type"` // "sampling" | "elicitation" | "tool_call" | "resource_access"
	Message string         `json:"message"`
	Context ApprovalContext `json:"context"`
	Timeout time.Duration  `json:"timeout,omitempty"`
}

type ApprovalContext struct {
	Server     string                 `json:"server"`
	Action     string                 `json:"action"`
	Parameters map[string]any         `json:"parameters,omitempty"`
	Security   *ApprovalSecurityContext `json:"security,omitempty"`
}

type ApprovalSecurityContext struct {
	RiskLevel string `json:"riskLevel"` // "low" | "medium" | "high"
	Reason    string `json:"reason"`
}

// HumanApprovalResponse models the decision returned by an approval handler.
type HumanApprovalResponse struct {
	Approved   bool     `json:"approved"`
	Reason     string   `json:"reason,omitempty"`
	Remember   bool     `json:"remember,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

// HumanApprovalHandler specifies the interface for handling approval requests.
type HumanApprovalHandler interface {
	RequestApproval(ctx context.Context, req HumanApprovalRequest) (HumanApprovalResponse, error)
}

// CreateElicitationRequest defines an elicitation payload.
type CreateElicitationRequest struct {
	Message         string         `json:"message"`
	RequestedSchema map[string]any `json:"requestedSchema,omitempty"`
	Timeout         time.Duration  `json:"timeout,omitempty"`
}

// CreateElicitationResult represents an elicitation response.
type CreateElicitationResult struct {
	Action  string `json:"action"` // "continue" | "retry" | "cancel"
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// CreateMessageRequest defines an LLM sampling message request.
type CreateMessageRequest struct {
	Messages         []SamplingMessage     `json:"messages"`
	ModelPreferences *ModelPreferences     `json:"modelPreferences,omitempty"`
	MaxTokens        int                   `json:"maxTokens,omitempty"`
}

type SamplingMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ModelPreferences struct {
	Hints []ModelHint `json:"hints,omitempty"`
}

type ModelHint struct {
	Name string `json:"name"`
}

// CreateMessageResult models sampling completion result.
type CreateMessageResult struct {
	Model   string `json:"model"`
	Content string `json:"content"`
	Role    string `json:"role"`
}

// MCPServerConfig defines configuration for an individual upstream MCP server.
type MCPServerConfig struct {
	Name         string            `json:"name"`
	Type         TransportType     `json:"type"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Cwd          string            `json:"cwd,omitempty"`
	URL          string            `json:"url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Timeout      time.Duration     `json:"timeout,omitempty"`
	Retries      int               `json:"retries,omitempty"`
	AllowedTools []string          `json:"allowedTools,omitempty"`
	Provider     string            `json:"provider,omitempty"`
	Category     string            `json:"category,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

// MCPClientConfig configures an MCP client connection.
type MCPClientConfig struct {
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
	Transport struct {
		Type      TransportType     `json:"type"`
		Command   string            `json:"command,omitempty"`
		Args      []string          `json:"args,omitempty"`
		Env       map[string]string `json:"env,omitempty"`
		Cwd       string            `json:"cwd,omitempty"`
		URL       string            `json:"url,omitempty"`
		Headers   map[string]string `json:"headers,omitempty"`
		Timeout   time.Duration     `json:"timeout,omitempty"`
		Retries   int               `json:"retries,omitempty"`
		KeepAlive bool              `json:"keepAlive,omitempty"`
	} `json:"transport"`
	Capabilities CompleteClientCapabilities `json:"capabilities"`
	Security     *SecurityPolicy            `json:"security,omitempty"`
	Tenant       *TenantContext             `json:"tenant,omitempty"`
	AllowedTools []string                   `json:"allowedTools,omitempty"`
}

// MCPAdapterConfig specifies top-level adapter aggregation options.
type MCPAdapterConfig struct {
	Servers        []MCPServerConfig                         `json:"servers"`
	DefaultTimeout time.Duration                             `json:"defaultTimeout,omitempty"`
	MaxRetries     int                                       `json:"maxRetries,omitempty"`
	OnError        func(err error, serverName string)        `json:"-"`
	ToolSecurity   *ToolSecurityConfig                       `json:"toolSecurity,omitempty"`
	ToolCache      *ToolCacheConfig                          `json:"toolCache,omitempty"`
}

type ToolSecurityConfig struct {
	RequireApproval []string            `json:"requireApproval,omitempty"`
	Timeouts        map[string]time.Duration `json:"timeouts,omitempty"`
	RateLimits      map[string]int      `json:"rateLimits,omitempty"`
	Permissions     map[string][]string `json:"permissions,omitempty"`
}

type ToolCacheConfig struct {
	Enabled  bool                     `json:"enabled"`
	TTLs     map[string]time.Duration `json:"ttls,omitempty"`
	Disabled []string                 `json:"disabled,omitempty"`
}

// MCPToolRaw represents a raw tool definition from an upstream MCP server.
type MCPToolRaw struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
}

// MCPToolRawWithServer includes server provenance.
type MCPToolRawWithServer struct {
	MCPToolRaw
	ServerName string `json:"serverName,omitempty"`
}

// MCPTool represents an executable MCP tool.
type MCPTool struct {
	MCPToolRaw
	Execute func(ctx context.Context, args map[string]any) (any, error) `json:"-"`
}

// MCPToolWithServer represents an executable tool bound to a specific server.
type MCPToolWithServer struct {
	MCPTool
	ServerName string `json:"serverName"`
}

// MCPResource defines a resource exposed by an MCP server.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceWithServer includes server provenance.
type MCPResourceWithServer struct {
	MCPResource
	ServerName string `json:"serverName"`
}

// MCPPrompt defines a prompt template exposed by an MCP server.
type MCPPrompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPPromptWithServer includes server provenance.
type MCPPromptWithServer struct {
	MCPPrompt
	ServerName string `json:"serverName"`
}

// EngineTool represents an agent-engine compatible tool definition.
type EngineTool struct {
	Name         string                                                      `json:"name"`
	Description  string                                                      `json:"description"`
	InputSchema  map[string]any                                              `json:"inputSchema"`
	OutputSchema map[string]any                                              `json:"outputSchema,omitempty"`
	Annotations  map[string]any                                              `json:"annotations,omitempty"`
	Title        string                                                      `json:"title,omitempty"`
	Execute      func(ctx context.Context, args map[string]any) (any, error) `json:"-"`
}

// MCPRegistryOptions configures the multi-server MCPRegistry.
type MCPRegistryOptions struct {
	DefaultTimeout time.Duration
	MaxRetries     int
	OnToolsChanged func(serverName string)
}

// MCPAdapter is the unified interface for coordinating remote MCP servers.
type MCPAdapter interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	EnsureConnection(ctx context.Context) error
	GetTools(ctx context.Context) ([]*MCPTool, error)
	HasTool(ctx context.Context, name string) (bool, error)
	ListResources(ctx context.Context) ([]MCPResourceWithServer, error)
	ReadResource(ctx context.Context, uri string, serverName ...string) (any, error)
	ListPrompts(ctx context.Context) ([]MCPPromptWithServer, error)
	GetPrompt(ctx context.Context, name string, args map[string]string, serverName ...string) (any, error)
	ExecuteTool(ctx context.Context, name string, args map[string]any, serverName ...string) (any, error)
	GetMetrics() map[string]any
	GetRegistry() any
}
