package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ResourceReader defines a function that resolves a resource URI to content.
type ResourceReader func(ctx context.Context, uri string) (string, error)

// MCPServer implements the Model Context Protocol server over JSON-RPC 2.0.
type MCPServer struct {
	mu              sync.RWMutex
	tools           []Tool
	handlers        map[string]ToolHandler
	resources       []Resource
	resourceReaders map[string]ResourceReader
	prompts         []PromptTemplate
}

// NewMCPServer initializes a server with built-in ScanDrix code review tools.
func NewMCPServer() *MCPServer {
	tools, handlers := GetBuiltinTools()

	s := &MCPServer{
		tools:           tools,
		handlers:        handlers,
		resources:       make([]Resource, 0),
		resourceReaders: make(map[string]ResourceReader),
		prompts:         make([]PromptTemplate, 0),
	}

	// Register default rule catalog resource
	s.RegisterResource(Resource{
		URI:         "scandrix://rules/catalog",
		Name:        "OWASP Security Catalog",
		Description: "Live static security rule definitions",
		MimeType:    "application/json",
	}, func(ctx context.Context, uri string) (string, error) {
		res, _ := handleCatalogSearch(ctx, map[string]any{})
		if len(res.Content) > 0 {
			return res.Content[0].Text, nil
		}
		return "[]", nil
	})

	return s
}

// RegisterResource adds a readable resource endpoint.
func (s *MCPServer) RegisterResource(res Resource, reader ResourceReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = append(s.resources, res)
	s.resourceReaders[res.URI] = reader
}

// RegisterTool adds a custom MCP tool.
func (s *MCPServer) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, tool)
	s.handlers[tool.Name] = handler
}

// HandleRequest processes an incoming JSON-RPC 2.0 message and returns the response.
func (s *MCPServer) HandleRequest(ctx context.Context, req JSONRPCRequest) *JSONRPCResponse {
	if req.JSONRPC != "2.0" {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &JSONRPCError{Code: CodeInvalidRequest, Message: "invalid jsonrpc version, must be '2.0'"},
		}
	}

	switch req.Method {
	case "initialize":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]string{
					"name":    "scandrix-mcp-gateway",
					"version": "1.0.0",
				},
				"capabilities": map[string]any{
					"tools":     map[string]bool{"listChanged": true},
					"resources": map[string]bool{"subscribe": false, "listChanged": true},
					"prompts":   map[string]bool{"listChanged": true},
				},
			},
		}

	case "tools/list":
		s.mu.RLock()
		tools := s.tools
		s.mu.RUnlock()
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": tools,
			},
		}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: CodeInvalidParams, Message: "failed parsing tool call parameters"},
			}
		}

		s.mu.RLock()
		handler, exists := s.handlers[params.Name]
		s.mu.RUnlock()

		if !exists {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: CodeMethodNotFound, Message: fmt.Sprintf("tool '%s' not found", params.Name)},
			}
		}

		result, err := handler(ctx, params.Arguments)
		if err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: ToolCallResult{
					IsError: true,
					Content: []ToolContent{{Type: "text", Text: err.Error()}},
				},
			}
		}

		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	case "resources/list":
		s.mu.RLock()
		res := s.resources
		s.mu.RUnlock()
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"resources": res,
			},
		}

	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: CodeInvalidParams, Message: "missing or invalid 'uri' parameter"},
			}
		}

		s.mu.RLock()
		reader, exists := s.resourceReaders[params.URI]
		s.mu.RUnlock()

		if !exists {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: CodeMethodNotFound, Message: fmt.Sprintf("resource '%s' not found", params.URI)},
			}
		}

		content, err := reader(ctx, params.URI)
		if err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: CodeInternalError, Message: err.Error()},
			}
		}

		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"contents": []map[string]string{
					{"uri": params.URI, "text": content},
				},
			},
		}

	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &JSONRPCError{Code: CodeMethodNotFound, Message: fmt.Sprintf("unsupported method: %s", req.Method)},
		}
	}
}
