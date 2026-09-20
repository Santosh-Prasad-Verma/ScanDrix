// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Server Enablement Guard
// File: mcp_enabled.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package guards

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// IsMcpServerEnabled checks if the MCP server feature flag is enabled.
// Defaults to true unless explicitly disabled via SCANDRIX_MCP_SERVER_ENABLED=false
// or API_MCP_SERVER_ENABLED=false.
func IsMcpServerEnabled() bool {
	val := os.Getenv("SCANDRIX_MCP_SERVER_ENABLED")
	if val == "" {
		val = os.Getenv("API_MCP_SERVER_ENABLED")
	}
	if val == "" {
		return true
	}
	val = strings.TrimSpace(strings.ToLower(val))
	return val != "false" && val != "0" && val != "off" && val != "disabled"
}

// McpEnabledMiddleware enforces that MCP server functionality is active.
// Returns HTTP 403 Forbidden when disabled.
func McpEnabledMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsMcpServerEnabled() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": http.StatusForbidden,
				"message":    "MCP Service is disabled",
				"error":      "Forbidden",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
