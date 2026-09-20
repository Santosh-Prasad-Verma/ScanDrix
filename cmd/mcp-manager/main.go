// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/mcp"
	"github.com/scandrix/backend/internal/mcp/manager/api"
	"github.com/scandrix/backend/internal/mcp/manager/config"
	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/custom"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
	"github.com/scandrix/backend/internal/mcp/manager/service"
)

func main() {
	stdioMode := flag.Bool("stdio", false, "Run in stdio JSON-RPC protocol mode for local agents")
	portFlag := flag.Int("port", 0, "Override HTTP port")
	flag.Parse()

	// 1. Direct agent stdio fallback (Cursor / Claude Code local pipe)
	if *stdioMode {
		mcp.RunCLI()
		return
	}

	// 2. Production HTTP Daemon Mode
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting ScanDrix MCP Manager Microservice (apps/mcp-manager equivalent)")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration failure", "error", err)
		os.Exit(1)
	}

	if *portFlag > 0 {
		cfg.Port = *portFlag
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Database connection & automated schema migrations
	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Warn("Database connection failed; running in transient catalog mode", "error", err)
	} else {
		defer dbPool.Close()
		slog.Info("Connected to PostgreSQL database. Running schema migrations...")
		if err := repository.EnsureSchemaAndTables(ctx, dbPool); err != nil {
			slog.Error("Failed ensuring mcp-manager schema", "error", err)
			os.Exit(1)
		}
		slog.Info("Schema 'mcp-manager' verified and up to date.")
	}

	// 4. Crypto & Repositories
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionSecret)
	if err != nil {
		slog.Error("Failed initializing cryptographic engine", "error", err)
		os.Exit(1)
	}

	repo := repository.NewMCPRepository(dbPool)
	integrationsSvc := service.NewIntegrationsService(repo, encryptor, cfg.RedirectURI)

	// 5. Providers Setup
	factory := providers.NewProviderFactory()
	scandrixProvider := scandrixmcp.NewScandrixMCPProvider(cfg.ServerBaseURL)
	customProvider := custom.NewCustomProvider(repo, encryptor)

	factory.Register(string(models.ProviderScandrixMCP), scandrixProvider)
	factory.Register(string(models.ProviderCustom), customProvider)

	// 6. Core Business Service & HTTP Handlers
	mcpSvc := service.NewMCPService(repo, factory, integrationsSvc, cfg.RedirectURI)
	handler := api.NewMCPHandler(mcpSvc, integrationsSvc)

	router := api.NewRouter(api.RouterConfig{
		Handler:      handler,
		JWTSecret:    cfg.JWTSecret,
		CORSOrigins:  cfg.CORSOrigins,
		DocsUser:     cfg.DocsUser,
		DocsPass:     cfg.DocsPass,
		DocsEnabled:  cfg.DocsEnabled,
		DocsPath:     cfg.DocsPath,
		DocsSpecPath: cfg.DocsSpecPath,
	})

	server := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("ScanDrix MCP Manager HTTP Server listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 7. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down ScanDrix MCP Manager...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Forced shutdown", "error", err)
	}

	slog.Info("ScanDrix MCP Manager gracefully stopped.")
}
