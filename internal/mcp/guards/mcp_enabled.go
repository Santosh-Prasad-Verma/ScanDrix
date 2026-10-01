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

// IsMcpServerEnabled reports whether the MCP server is enabled.
//
// SECURITY (AUDIT_REMEDIATION.md F-15f): this previously returned true when
// the variable was unset, so every deployment served an MCP endpoint that let
// any caller reach tools with a self-asserted organizationId. The default is
// now CLOSED: a deployment must opt in explicitly by setting
// SCANDRIX_MCP_SERVER_ENABLED=true (or API_MCP_SERVER_ENABLED=true).
//
// This follows the same posture as the registration and cookie flags, where an
// unset variable is a denial rather than a grant.
func IsMcpServerEnabled() bool {
	val := os.Getenv("SCANDRIX_MCP_SERVER_ENABLED")
	if val == "" {
		val = os.Getenv("API_MCP_SERVER_ENABLED")
	}
	if strings.TrimSpace(val) == "" {
		return false
	}
	val = strings.ToLower(strings.TrimSpace(val))
	return val == "true" || val == "1" || val == "on" || val == "enabled"
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
