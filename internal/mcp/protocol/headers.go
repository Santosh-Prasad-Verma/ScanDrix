// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Server Protocol Headers
// File: headers.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package protocol

import (
	"net/http"
	"net/url"
	"strings"
)

// MCPExposedHeaders lists headers exposed to CORS clients.
var MCPExposedHeaders = []string{
	"Mcp-Session-Id",
	"Mcp-Protocol-Version",
	"Last-Event-ID",
}

// ApplyMcpHttpResponseHeaders sets the CORS exposed headers for Streamable HTTP MCP clients.
func ApplyMcpHttpResponseHeaders(w http.ResponseWriter) {
	if w == nil {
		return
	}
	w.Header().Set("Access-Control-Expose-Headers", strings.Join(MCPExposedHeaders, ", "))
}

// NormalizeOrigin extracts the scheme and host from an origin string.
func NormalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// IsAllowedMcpOrigin verifies if an incoming origin matches the allowed configured origin.
func IsAllowedMcpOrigin(allowedOrigin, requestOrigin string) bool {
	normAllowed := NormalizeOrigin(allowedOrigin)
	if normAllowed == "" {
		return allowedOrigin == ""
	}

	normReq := NormalizeOrigin(requestOrigin)
	return normReq != "" && normAllowed == normReq
}
