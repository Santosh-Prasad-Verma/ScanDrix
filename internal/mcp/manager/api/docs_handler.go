// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// OpenAPISpecHandler returns OpenAPI 3.0 JSON specification for the MCP Manager.
func OpenAPISpecHandler(w http.ResponseWriter, r *http.Request) {
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "ScanDrix MCP Manager API",
			"description": "Model Context Protocol Connection & Integration Manager - ScanDrix AI Enterprise Platform",
			"version":     "1.0.0",
		},
		"servers": []map[string]string{
			{"url": "/"},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]string{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		},
		"security": []map[string][]string{
			{"bearerAuth": {}},
		},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": map[string]any{
					"summary":     "Liveness Probe",
					"description": "Health status of MCP Manager microservice",
					"responses": map[string]any{
						"200": map[string]string{"description": "Healthy"},
					},
				},
			},
			"/mcp/connections": map[string]any{
				"get": map[string]any{
					"summary":     "List Connections",
					"description": "Lists MCP connections for authenticated organization.",
					"responses": map[string]any{
						"200": map[string]string{"description": "Paginated list of connections"},
						"401": map[string]string{"description": "Unauthorized"},
					},
				},
				"patch": map[string]any{
					"summary":     "Update Connection",
					"description": "Updates connection status or metadata by integration ID.",
					"responses": map[string]any{
						"200": map[string]string{"description": "Updated connection"},
					},
				},
			},
			"/mcp/connections/{connectionId}": map[string]any{
				"get": map[string]any{
					"summary": "Get Connection",
					"responses": map[string]any{
						"200": map[string]string{"description": "Connection details"},
						"404": map[string]string{"description": "Not Found"},
					},
				},
				"delete": map[string]any{
					"summary": "Delete Connection",
					"responses": map[string]any{
						"200": map[string]string{"description": "Deleted"},
					},
				},
			},
			"/mcp/integrations": map[string]any{
				"get": map[string]any{
					"summary": "List Available Integrations",
					"responses": map[string]any{
						"200": map[string]string{"description": "List of catalog integrations"},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}

// SwaggerUIHandler serves a responsive Swagger UI documentation interface.
func SwaggerUIHandler(docsSpecPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>ScanDrix MCP Manager - API Documentation</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body style="margin: 0; background: #fafafa;">
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        window.onload = () => {
            window.ui = SwaggerUIBundle({
                url: '%s',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ],
                supportedSubmitMethods: []
            });
        };
    </script>
</body>
</html>`, docsSpecPath)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}
}
