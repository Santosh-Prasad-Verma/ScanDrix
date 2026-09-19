// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Streamable HTTP Server
// File: http_server.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/mcp/guards"
	"github.com/scandrix/backend/internal/mcp/protocol"
)

// HTTPServer provides stateless streamable HTTP transport for the MCP server.
type HTTPServer struct {
	server *Server
	router http.Handler
}

// NewHTTPServer constructs an HTTP router serving MCP over Streamable HTTP POST.
func NewHTTPServer(server *Server) http.Handler {
	if server == nil {
		server = NewServer()
	}

	hs := &HTTPServer{server: server}
	r := chi.NewRouter()

	// 1. Guard check: SCANDRIX_MCP_SERVER_ENABLED / API_MCP_SERVER_ENABLED
	r.Use(guards.McpEnabledMiddleware)

	// 2. Main Code Management MCP endpoints on /mcp
	// Supports both root (when mounted at /mcp) and explicit /mcp paths
	r.Post("/", hs.handleMCP)
	r.Get("/", hs.handleMethodNotAllowed)
	r.Delete("/", hs.handleMethodNotAllowed)

	r.Post("/mcp", hs.handleMCP)
	r.Get("/mcp", hs.handleMethodNotAllowed)
	r.Delete("/mcp", hs.handleMethodNotAllowed)

	// 3. ScanDrix Issues MCP endpoints on /mcp/issues
	r.Post("/issues", hs.handleIssuesMCP)
	r.Get("/issues", hs.handleMethodNotAllowed)
	r.Delete("/issues", hs.handleMethodNotAllowed)

	r.Post("/mcp/issues", hs.handleIssuesMCP)
	r.Get("/mcp/issues", hs.handleMethodNotAllowed)
	r.Delete("/mcp/issues", hs.handleMethodNotAllowed)

	// Backward compatibility alias for ScanDrix legacy tests
	r.Post("/scandrix-issues", hs.handleIssuesMCP)
	r.Get("/scandrix-issues", hs.handleMethodNotAllowed)
	r.Delete("/scandrix-issues", hs.handleMethodNotAllowed)

	r.Post("/mcp/scandrix-issues", hs.handleIssuesMCP)
	r.Get("/mcp/scandrix-issues", hs.handleMethodNotAllowed)
	r.Delete("/mcp/scandrix-issues", hs.handleMethodNotAllowed)

	hs.router = r
	return hs
}

// ServeHTTP delegates to the internal Chi router.
func (h *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

// handleMethodNotAllowed returns standard 405 Method Not Allowed response.
func (h *HTTPServer) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	protocol.ApplyMcpHttpResponseHeaders(w)
	w.Header().Set("Allow", "POST")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)

	resp := protocol.JSONRPCErrorResponse{
		JSONRPC: "2.0",
		ID:      nil,
		Error: protocol.JSONRPCError{
			Code:    int(protocol.ServerError),
			Message: "Method not allowed.",
			Data: map[string]any{
				"name": "Error",
			},
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleMCP handles POST requests to the main code-management MCP server.
func (h *HTTPServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	h.processRequest(w, r, h.server.HandleMethod, "McpController")
}

// handleIssuesMCP handles POST requests to the issues MCP server.
func (h *HTTPServer) handleIssuesMCP(w http.ResponseWriter, r *http.Request) {
	h.processRequest(w, r, h.server.HandleIssuesMethod, "IssuesMcpController")
}

func (h *HTTPServer) processRequest(w http.ResponseWriter, r *http.Request, handler func(ctx context.Context, req JSONRPCRequest) JSONRPCResponse, controllerName string) {
	start := time.Now()
	protocol.ApplyMcpHttpResponseHeaders(w)

	// Limit request payload to 10MB
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10*1024*1024))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(protocol.ToJSONRPCError(
			fmt.Errorf("Request entity too large or read error: %w", err),
			nil,
		))
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(protocol.JSONRPCErrorResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: protocol.JSONRPCError{
				Code:    int(protocol.ParseError),
				Message: "Parse error",
			},
		})
		return
	}

	reqMeta := ExtractMcpRequestMetadata(req)
	slog.Info("MCP stateless request received",
		"method", r.Method,
		"path", r.URL.Path,
		"controller", controllerName,
		"instanceId", h.server.instanceID,
		"metadata", reqMeta,
	)

	// Listen for client aborts / context cancellations
	ctx := r.Context()
	doneChan := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				slog.Warn("MCP stateless request aborted",
					"method", r.Method,
					"path", r.URL.Path,
					"latencyMs", time.Since(start).Milliseconds(),
					"instanceId", h.server.instanceID,
					"metadata", reqMeta,
				)
			}
		case <-doneChan:
		}
	}()
	defer close(doneChan)

	resp := handler(ctx, req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)

	slog.Info("MCP stateless request completed",
		"method", r.Method,
		"path", r.URL.Path,
		"statusCode", http.StatusOK,
		"latencyMs", time.Since(start).Milliseconds(),
		"instanceId", h.server.instanceID,
		"metadata", reqMeta,
	)
}
