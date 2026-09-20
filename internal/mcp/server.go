// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Server Engine
// File: server.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/mcp/protocol"
	"github.com/scandrix/backend/internal/mcp/tools"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
)

// JSONRPCRequest represents an incoming MCP JSON-RPC 2.0 message.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// JSONRPCResponse represents an outgoing MCP JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// Server provides the Model Context Protocol engine over stdio or HTTP.
type Server struct {
	evaluator   *rules.Evaluator
	mu          sync.RWMutex
	tools       map[string]tools.MCPTool
	toolList    []tools.MCPTool
	issuesTools map[string]tools.MCPTool
	issuesList  []tools.MCPTool
	instanceID  string
}

// NewServer initializes the MCP server with complete tool suites.
func NewServer() *Server {
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	host, _ := os.Hostname()
	if host == "" {
		host = "scandrix-mcp-node"
	}

	s := &Server{
		evaluator:   evaluator,
		tools:       make(map[string]tools.MCPTool),
		issuesTools: make(map[string]tools.MCPTool),
		instanceID:  host,
	}

	// 1. Register Code Management tools
	for _, t := range tools.GetCodeManagementTools() {
		s.registerTool(t)
	}

	// 2. Register ScanDrix review finding issues tools
	for _, t := range tools.GetReviewIssuesTools() {
		s.registerTool(t)
	}

	// 3. Register Drixy Rules and Memories tools
	for _, t := range tools.GetDrixyRulesTools() {
		s.registerTool(t)
	}

	// 4. Register SCM issue tools dedicated to /mcp/issues
	for _, t := range tools.GetSCMIssuesTools() {
		s.issuesTools[t.Name] = t
		s.issuesList = append(s.issuesList, t)
	}

	// 5. Register diff review tools
	s.registerTool(tools.MCPTool{
		Name:        "drixy_review_diff",
		Description: "Trigger an autonomous Drixy AI code review on a git unified diff.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"diff": map[string]string{
					"type":        "string",
					"description": "Unified git diff string to analyze",
				},
			},
			"required": []string{"diff"},
		},
		Handler: s.handleDiffReview,
	})

	s.registerTool(tools.MCPTool{
		Name:        "scandrix_review_diff",
		Description: "Analyze a git unified diff for security vulnerabilities, bugs, and compliance defects.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"diff": map[string]string{
					"type":        "string",
					"description": "Unified git diff string to analyze",
				},
			},
			"required": []string{"diff"},
		},
		Handler: s.handleDiffReview,
	})

	s.registerTool(tools.MCPTool{
		Name:        "drixy_list_rules",
		Description: "List all active Drixy static security and quality rules (.drixy/rules/).",
		InputSchema: map[string]any{"type": "object"},
		Handler:     s.handleListRules,
	})

	s.registerTool(tools.MCPTool{
		Name:        "scandrix_list_rules",
		Description: "List all active static security and defect rules currently loaded in ScanDrix.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     s.handleListRules,
	})

	return s
}

func (s *Server) registerTool(t tools.MCPTool) {
	s.tools[t.Name] = t
	s.toolList = append(s.toolList, t)
}

func (s *Server) handleDiffReview(ctx context.Context, args map[string]any) (any, error) {
	diffStr, _ := args["diff"].(string)
	if diffStr == "" {
		return nil, fmt.Errorf("missing diff argument")
	}

	patches, err := diff.ParseUnifiedDiff(strings.NewReader(diffStr))
	if err != nil {
		return nil, fmt.Errorf("failed parsing diff: %w", err)
	}

	findings := s.evaluator.EvaluatePatches(uuid.New(), uuid.New(), patches)
	return map[string]any{
		"verdict":  "EVALUATED",
		"findings": findings,
	}, nil
}

func (s *Server) handleListRules(ctx context.Context, args map[string]any) (any, error) {
	return rules.DefaultCatalog(), nil
}

// ExtractMcpRequestMetadata extracts audit and tracing fields from a JSON-RPC request.
func ExtractMcpRequestMetadata(req JSONRPCRequest) map[string]any {
	meta := map[string]any{
		"jsonrpcMethod": req.Method,
	}

	if len(req.Params) > 0 {
		var p struct {
			Name      string         `json:"name"`
			RequestID string         `json:"requestId"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err == nil {
			if p.Name != "" {
				meta["toolName"] = p.Name
			}
			if p.RequestID != "" {
				meta["requestId"] = p.RequestID
			}
			if p.Arguments != nil {
				if orgID, ok := p.Arguments["organizationId"].(string); ok && orgID != "" {
					meta["organizationId"] = orgID
				}
				if teamID, ok := p.Arguments["teamId"].(string); ok && teamID != "" {
					meta["teamId"] = teamID
				}
				if reqID, ok := p.Arguments["requestId"].(string); ok && reqID != "" {
					meta["requestId"] = reqID
				}
			}
		}
	}

	return meta
}

// HandleMethod dispatches a JSON-RPC 2.0 request against code management & review tools.
func (s *Server) HandleMethod(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	return s.dispatch(ctx, req, s.toolList, s.tools, "scandrix-code-management")
}

// HandleIssuesMethod dispatches a JSON-RPC 2.0 request against SCM Issues tools.
func (s *Server) HandleIssuesMethod(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	return s.dispatch(ctx, req, s.issuesList, s.issuesTools, "issues-by-scandrix")
}

func (s *Server) dispatch(ctx context.Context, req JSONRPCRequest, toolCatalog []tools.MCPTool, toolMap map[string]tools.MCPTool, serverName string) JSONRPCResponse {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    serverName,
				"version": "1.0.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]bool{"listChanged": false},
			},
		}

	case "ping":
		resp.Result = map[string]any{}

	case "tools/list":
		var wireTools []map[string]any
		for _, t := range toolCatalog {
			wireTools = append(wireTools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		resp.Result = map[string]any{
			"tools": wireTools,
		}

	case "tools/call":
		var callParams struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			resp.Error = map[string]any{
				"code":    int(protocol.InvalidParams),
				"message": "Invalid params",
			}
			return resp
		}

		targetTool, ok := toolMap[callParams.Name]
		if !ok {
			resp.Error = map[string]any{
				"code":    int(protocol.MethodNotFound),
				"message": fmt.Sprintf("Tool %s not found", callParams.Name),
			}
			return resp
		}

		start := time.Now()
		slog.Info("MCP tool invoked",
			"tool", callParams.Name,
			"organizationId", callParams.Arguments["organizationId"],
			"teamId", callParams.Arguments["teamId"],
		)

		result, err := targetTool.Handler(ctx, callParams.Arguments)
		duration := time.Since(start)

		if err != nil {
			slog.Error("MCP tool failed",
				"tool", callParams.Name,
				"durationMs", duration.Milliseconds(),
				"error", err,
			)
			errPayload := protocol.ToToolErrorPayload(err)
			errBytes, _ := json.Marshal(errPayload)
			resp.Result = map[string]any{
				"content": []map[string]string{
					{
						"type": "text",
						"text": string(errBytes),
					},
				},
				"isError": true,
			}
			return resp
		}

		slog.Info("MCP tool completed",
			"tool", callParams.Name,
			"durationMs", duration.Milliseconds(),
		)

		resultBytes, _ := json.MarshalIndent(result, "", "  ")
		resp.Result = map[string]any{
			"content": []map[string]string{
				{
					"type": "text",
					"text": string(resultBytes),
				},
			},
			"structuredContent": result,
		}

	default:
		resp.Error = map[string]any{
			"code":    int(protocol.MethodNotFound),
			"message": fmt.Sprintf("Method %s not found", req.Method),
		}
	}

	return resp
}

// ServeStdio executes the MCP JSON-RPC 2.0 protocol over standard input/output.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // Support up to 10MB payload
	encoder := json.NewEncoder(w)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = encoder.Encode(protocol.ToJSONRPCError(err, nil))
			continue
		}

		resp := s.HandleMethod(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// RunCLI launches the MCP server on stdio.
func RunCLI() {
	server := NewServer()
	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server exited: %v\n", err)
		os.Exit(1)
	}
}
