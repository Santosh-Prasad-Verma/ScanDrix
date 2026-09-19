// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
)

// RouterConfig defines settings for wiring the MCP Manager HTTP router.
type RouterConfig struct {
	Handler      *MCPHandler
	Pool         *pgxpool.Pool
	JWTSecret    string
	CORSOrigins  []string
	DocsUser     string
	DocsPass     string
	DocsEnabled  bool
	DocsPath     string
	DocsSpecPath string
}

// NewRouter constructs a complete production Chi HTTP router for the MCP Manager.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Base middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS configuration
	allowedOrigins := cfg.CORSOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = scandrixMiddleware.DefaultCORSConfig().AllowedOrigins
	}
	r.Use(scandrixMiddleware.CORS(scandrixMiddleware.CORSConfig{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
	}))

	// 1. Health probe with live DB ping
	r.Get("/health", NewHealthHandler(cfg.Pool))

	// 2. Interactive OpenAPI Documentation & Swagger UI
	docsPath := cfg.DocsPath
	if docsPath == "" {
		docsPath = "/docs"
	}
	if !strings.HasPrefix(docsPath, "/") {
		docsPath = "/" + docsPath
	}
	docsSpecPath := cfg.DocsSpecPath
	if docsSpecPath == "" {
		docsSpecPath = "/openapi.json"
	}
	if !strings.HasPrefix(docsSpecPath, "/") {
		docsSpecPath = "/" + docsSpecPath
	}

	if cfg.DocsEnabled {
		r.Group(func(docs chi.Router) {
			docs.Use(DocsBasicAuth(cfg.DocsUser, cfg.DocsPass))
			docs.Get(docsSpecPath, OpenAPISpecHandler)
			docs.Get(docsPath, SwaggerUIHandler(docsSpecPath))
			docs.Get(docsPath+"/*", SwaggerUIHandler(docsSpecPath))
		})
	}

	// 3. Authenticated MCP APIs
	h := cfg.Handler
	r.Route("/mcp", func(mcp chi.Router) {
		mcp.Use(AuthGuard(cfg.JWTSecret))

		// Connections
		mcp.Get("/connections", h.GetConnections)
		mcp.Get("/connections/{connectionId}", h.GetConnection)
		mcp.Patch("/connections", h.UpdateConnection)
		mcp.Delete("/connections/{connectionId}", h.DeleteConnection)
		mcp.Put("/connections/{integrationId}/allowed-tools", h.UpdateAllowedTools)

		// Catalog
		mcp.Get("/integrations", h.GetIntegrations)
		mcp.Get("/{provider}/integrations/{integrationId}", h.GetIntegration)
		mcp.Get("/{provider}/integrations/{integrationId}/required-params", h.GetIntegrationRequiredParams)
		mcp.Get("/{provider}/integrations/{integrationId}/tools", h.GetIntegrationTools)
		mcp.Post("/{provider}/connect", h.InitiateConnection)

		// Bring-Your-Own-Token (scandrixmcp)
		mcp.Post("/integration/scandrixmcp/{integrationId}/token", h.ConnectManagedToken)
		mcp.Get("/integration/scandrixmcp/{integrationId}/connection-config", h.GetManagedConnectionConfig)

		// Custom Integrations
		mcp.Get("/integration/custom", h.GetCustomIntegrations)
		mcp.Get("/integration/custom/{integrationId}", h.GetCustomIntegration)
		mcp.Get("/integration/custom/{integrationId}/connection-config", h.GetCustomIntegrationConnectionConfig)

		// Dynamic provider CRUD
		mcp.Post("/integration/{provider}", h.CreateIntegration)
		mcp.Put("/integration/{provider}/{integrationId}", h.EditIntegration)
		mcp.Delete("/integration/{provider}/{integrationId}", h.DeleteIntegration)

		// OAuth lifecycle
		mcp.Post("/integration/{provider}/oauth/initialize", h.InitializeOAuthIntegration)
		mcp.Post("/integration/{provider}/oauth/finalize", h.FinalizeOAuthIntegration)
	})

	return r
}
