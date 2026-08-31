package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
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
	evaluator *rules.Evaluator
}

// NewServer initializes the MCP server with the default security rules catalog.
func NewServer() *Server {
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	return &Server{evaluator: evaluator}
}

// ServeStdio executes the MCP JSON-RPC 2.0 protocol over standard input/output.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // Support up to 10MB payload / unified diff lines
	encoder := json.NewEncoder(w)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = encoder.Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   map[string]any{"code": -32700, "message": "Parse error"},
			})
			continue
		}

		resp := s.handleMethod(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func (s *Server) handleMethod(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "scandrix-mcp-manager",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]bool{"listChanged": false},
			},
		}

	case "tools/list":
		resp.Result = map[string]any{
			"tools": []map[string]any{
				{
					"name":        "scandrix_review_diff",
					"description": "Analyze a git unified diff for security vulnerabilities, bugs, and compliance defects.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"diff": map[string]string{
								"type":        "string",
								"description": "Unified git diff string to analyze",
							},
						},
						"required": []string{"diff"},
					},
				},
				{
					"name":        "scandrix_list_rules",
					"description": "List all active static security and defect rules currently loaded in Scandrix.",
					"inputSchema": map[string]any{
						"type": "object",
					},
				},
			},
		}

	case "tools/call":
		var callParams struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			resp.Error = map[string]any{"code": -32602, "message": "Invalid params"}
			return resp
		}

		switch callParams.Name {
		case "scandrix_review_diff":
			diffStr, _ := callParams.Arguments["diff"].(string)
			if diffStr == "" {
				resp.Error = map[string]any{"code": -32602, "message": "Missing diff argument"}
				return resp
			}

			patches, err := diff.ParseUnifiedDiff(strings.NewReader(diffStr))
			if err != nil {
				resp.Error = map[string]any{"code": -32000, "message": fmt.Sprintf("Failed parsing diff: %v", err)}
				return resp
			}

			findings := s.evaluator.EvaluatePatches(uuid.New(), uuid.New(), patches)
			findingsJSON, _ := json.MarshalIndent(findings, "", "  ")

			resp.Result = map[string]any{
				"content": []map[string]string{
					{
						"type": "text",
						"text": string(findingsJSON),
					},
				},
			}

		case "scandrix_list_rules":
			rulesCatalog := rules.DefaultCatalog()
			catalogJSON, _ := json.MarshalIndent(rulesCatalog, "", "  ")
			resp.Result = map[string]any{
				"content": []map[string]string{
					{
						"type": "text",
						"text": string(catalogJSON),
					},
				},
			}

		default:
			resp.Error = map[string]any{"code": -32601, "message": "Method not found"}
		}

	default:
		resp.Error = map[string]any{"code": -32601, "message": "Method not found"}
	}

	return resp
}

// RunCLI launches the MCP server on stdio.
func RunCLI() {
	server := NewServer()
	if err := server.ServeStdio(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server exited: %v\n", err)
		os.Exit(1)
	}
}
